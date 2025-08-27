package dlp

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// CheckRaw scans one outbound message against the enabled rules for the
// sender's domain and returns the verdict. A match with action block
// rejects the submission; action hold stores the message and notifies the
// approvers. No match returns pass.
func (s *Service) CheckRaw(ctx context.Context, senderEmail, from string, to []string, raw []byte) (*models.DlpCheckResult, error) {
	domain := domainOf(from)
	var rules []models.DlpRule
	if err := s.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rules).Error; err != nil {
		return nil, err
	}
	var scoped []models.DlpRule
	for _, r := range rules {
		if ruleApplies(r, domain) {
			scoped = append(scoped, r)
		}
	}
	if len(scoped) == 0 {
		return &models.DlpCheckResult{Action: "pass"}, nil
	}
	re, index, err := buildMatcher(scoped)
	if err != nil {
		// A broken rule must not take mail down: treat as no match and log.
		return &models.DlpCheckResult{Action: "pass"}, nil
	}
	if re == nil {
		return &models.DlpCheckResult{Action: "pass"}, nil
	}
	text := scanText(raw)
	m := re.FindStringSubmatchIndex(text)
	if m == nil {
		return &models.DlpCheckResult{Action: "pass"}, nil
	}
	// Map the first matching group back to its rule. Block wins over hold.
	matched := -1
	for i := 1; i < len(m); i += 2 {
		if m[i] >= 0 {
			matched = index[i/2]
			break
		}
	}
	if matched < 0 {
		return &models.DlpCheckResult{Action: "pass"}, nil
	}
	rule := scoped[matched]
	reason := fmt.Sprintf("命中敏感规则「%s」", rule.Name)
	if rule.Action == models.DlpActionBlock {
		return &models.DlpCheckResult{Action: "block", Reason: reason}, nil
	}
	// Hold: persist for review and notify the designated approvers.
	p, err := s.holdMessage(ctx, senderEmail, from, to, raw, rule)
	if err != nil {
		// A storage failure must not silently drop mail: fail open.
		return &models.DlpCheckResult{Action: "pass"}, nil
	}
	return &models.DlpCheckResult{Action: "hold", Reason: reason, ID: p.ID}, nil
}

// holdMessage persists a held message and emails the approvers.
func (s *Service) holdMessage(ctx context.Context, senderEmail, from string, to []string, raw []byte, rule models.DlpRule) (*models.PendingApproval, error) {
	hours := rule.HoldHours
	if hours <= 0 {
		hours = 48
	}
	expires := time.Now().Add(time.Duration(hours) * time.Hour)
	p := &models.PendingApproval{
		RuleName:    rule.Name,
		Severity:    rule.Severity,
		SenderEmail: senderEmail,
		From:        from,
		Recipients:  strings.Join(to, ", "),
		Subject:     subjectOf(raw),
		Raw:         raw,
		Status:      "pending",
		ExpiresAt:   &expires,
	}
	if err := s.DB.WithContext(ctx).Create(p).Error; err != nil {
		return nil, err
	}
	for _, a := range splitApprovers(rule.Approvers) {
		if err := s.sendApprovalNotice(ctx, a, p, rule); err != nil {
			// Notification failure must not block the hold itself.
			logNotify("approval notice", a, p.ID, err)
		}
	}
	return p, nil
}

// stackCheck is the engine-facing scan endpoint (multipart meta + raw).
func (s *Service) stackCheck(c *fiber.Ctx) error {
	var ev struct {
		SenderEmail string   `json:"sender_email"`
		From        string   `json:"from"`
		To          []string `json:"to"`
	}
	if err := parseMeta(c.FormValue("meta"), &ev); err != nil {
		return core.Fail(c, 400, err, "bad meta")
	}
	rawFile, err := c.FormFile("raw")
	if err != nil {
		return core.Fail(c, 400, err, "missing raw part")
	}
	f, err := rawFile.Open()
	if err != nil {
		return core.Fail(c, 400, err, "bad raw part")
	}
	raw, err := readLimited(f, 64<<20)
	f.Close()
	if err != nil {
		return core.Fail(c, 400, err, "read raw")
	}
	res, err := s.CheckRaw(c.Context(), ev.SenderEmail, ev.From, ev.To, raw)
	if err != nil {
		// Fail open on backend-side errors: never reject mail because the
		// DLP store is unavailable.
		return c.JSON(models.DlpCheckResult{Action: "pass"})
	}
	return c.JSON(res)
}

func parseMeta(v string, out any) error {
	if v == "" {
		return errors.New("missing meta")
	}
	return jsonUnmarshal([]byte(v), out)
}

func splitApprovers(v string) []string {
	var out []string
	for _, a := range strings.Split(v, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func domainOf(addr string) string {
	a, err := mail.ParseAddress(addr)
	if err != nil {
		return ""
	}
	_, d, ok := strings.Cut(a.Address, "@")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(d))
}
