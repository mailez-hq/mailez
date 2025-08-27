package dlp

import (
	"context"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Service owns DLP rules and pending approvals.
type Service struct {
	*core.App
	// send overrides the SMTP delivery used for approved mail and notices
	// (tests inject a fake; production defaults to the local MTA).
	send func(from string, to []string, raw []byte) error
}

// New builds the DLP service.
func New(app *core.App) *Service {
	return &Service{App: app}
}

// Register mounts the admin rule CRUD (global admin) and the approval
// workflow (designated approvers or global admins).
func (s *Service) Register(r fiber.Router) {
	r.Get("/dlp/rules", s.RequireGlobalAdmin, s.listRules)
	r.Post("/dlp/rules", s.RequireGlobalAdmin, s.createRule)
	r.Put("/dlp/rules/:id", s.RequireGlobalAdmin, s.updateRule)
	r.Delete("/dlp/rules/:id", s.RequireGlobalAdmin, s.deleteRule)
	r.Get("/dlp/approvals", s.requireApprover, s.listApprovals)
	r.Get("/dlp/approvals/:id", s.requireApprover, s.getApproval)
	r.Post("/dlp/approvals/:id/decision", s.requireApprover, s.decide)
}

// RegisterStack mounts the engine scan endpoint.
func (s *Service) RegisterStack(r fiber.Router) {
	r.Post("/dlp/check", s.stackCheck)
}

// RunExpiry auto-rejects approvals past their expiry deadline so held mail
// never waits forever; the sender is notified once.
func (s *Service) RunExpiry(ctx context.Context) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	run := func() {
		var due []models.PendingApproval
		if err := s.DB.WithContext(ctx).
			Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", "pending", time.Now()).
			Find(&due).Error; err != nil {
			log.Printf("dlp expiry: %v", err)
			return
		}
		now := time.Now()
		for i := range due {
			p := &due[i]
			p.Status = "rejected"
			p.Reason = "审批超时自动拒绝"
			p.Approver = "system"
			p.DecisionAt = &now
			if err := s.DB.WithContext(ctx).Save(p).Error; err != nil {
				log.Printf("dlp expiry: save %d: %v", p.ID, err)
				continue
			}
			s.notifySender(p, "expired")
			s.markOutboxRejected(p.ID, "审批超时自动拒绝")
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// requireApprover allows global admins and users listed as approvers on any
// rule. Per-item checks happen in each handler.
func (s *Service) requireApprover(c *fiber.Ctx) error {
	u := core.CurrentUser(c)
	if u == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
	if u.GlobalAdmin {
		return c.Next()
	}
	if s.isApprover(u.Email) {
		return c.Next()
	}
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "approver required"})
}

func (s *Service) isApprover(email string) bool {
	var rules []models.DlpRule
	if err := s.DB.Where("action = ? AND approvers <> ?", models.DlpActionHold, "").Find(&rules).Error; err != nil {
		return false
	}
	lower := strings.ToLower(email)
	for _, r := range rules {
		for _, a := range strings.Split(r.Approvers, ",") {
			if strings.EqualFold(strings.TrimSpace(a), lower) {
				return true
			}
		}
	}
	return false
}

// canApprove reports whether the user may decide on a pending item.
func (s *Service) canApprove(c *fiber.Ctx, p *models.PendingApproval) bool {
	u := core.CurrentUser(c)
	if u == nil {
		return false
	}
	if u.GlobalAdmin {
		return true
	}
	var rule models.DlpRule
	if err := s.DB.Where("name = ?", p.RuleName).First(&rule).Error; err != nil {
		return false
	}
	for _, a := range strings.Split(rule.Approvers, ",") {
		if strings.EqualFold(strings.TrimSpace(a), u.Email) {
			return true
		}
	}
	return false
}

// subjectOf extracts the first Subject header.
func subjectOf(raw []byte) string {
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(msg.Header.Get("Subject"))
}
