package dlp

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (s *Service) listRules(c *fiber.Ctx) error {
	var rules []models.DlpRule
	if err := s.DB.WithContext(c.Context()).Order("id").Find(&rules).Error; err != nil {
		return core.Fail(c, 500, err, "load rules failed")
	}
	return c.JSON(fiber.Map{"data": rules})
}

func (s *Service) createRule(c *fiber.Ctx) error {
	var body models.DlpRule
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	if err := validateRule(&body); err != nil {
		return core.Fail(c, 400, err, err.Error())
	}
	if err := s.DB.WithContext(c.Context()).Create(&body).Error; err != nil {
		return core.Fail(c, 500, err, "create rule failed")
	}
	return c.JSON(body)
}

func (s *Service) updateRule(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	var rule models.DlpRule
	if err := s.DB.WithContext(c.Context()).First(&rule, id).Error; err != nil {
		return core.Fail(c, 404, err, "rule not found")
	}
	var body models.DlpRule
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	if body.Name != "" {
		rule.Name = body.Name
	}
	if body.Pattern != "" {
		rule.Pattern = body.Pattern
	}
	rule.Enabled = body.Enabled
	rule.IsRegex = body.IsRegex
	if body.Scope != "" {
		rule.Scope = body.Scope
	}
	if body.Action != "" {
		rule.Action = body.Action
	}
	if body.Severity != "" {
		rule.Severity = body.Severity
	}
	if body.Approvers != "" {
		rule.Approvers = body.Approvers
	}
	if body.HoldHours > 0 {
		rule.HoldHours = body.HoldHours
	}
	rule.Note = body.Note
	if err := validateRule(&rule); err != nil {
		return core.Fail(c, 400, err, err.Error())
	}
	if err := s.DB.WithContext(c.Context()).Save(&rule).Error; err != nil {
		return core.Fail(c, 500, err, "save rule failed")
	}
	return c.JSON(rule)
}

func (s *Service) deleteRule(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	res := s.DB.WithContext(c.Context()).Delete(&models.DlpRule{}, id)
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "delete rule failed")
	}
	if res.RowsAffected == 0 {
		return core.Fail(c, 404, nil, "rule not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func validateRule(r *models.DlpRule) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Pattern = strings.TrimSpace(r.Pattern)
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Name == "" || r.Pattern == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and pattern are required")
	}
	if r.Scope == "" {
		r.Scope = "all"
	}
	if r.Action != models.DlpActionBlock && r.Action != models.DlpActionHold {
		return fiber.NewError(fiber.StatusBadRequest, "action must be block or hold")
	}
	if r.Action == models.DlpActionHold && strings.TrimSpace(r.Approvers) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "hold rules require at least one approver")
	}
	if r.IsRegex {
		if _, err := regexp.Compile("(?i)" + r.Pattern); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid regex: "+err.Error())
		}
	}
	if r.Severity == "" {
		r.Severity = "medium"
	}
	if r.HoldHours <= 0 {
		r.HoldHours = 48
	}
	return nil
}

// listApprovals returns pending (default) or all approvals the user may
// decide on, newest first.
func (s *Service) listApprovals(c *fiber.Ctx) error {
	u := core.CurrentUser(c)
	page, limit := core.PageParams(c)
	status := c.Query("status", "pending")
	q := s.DB.WithContext(c.Context()).Model(&models.PendingApproval{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if !u.GlobalAdmin {
		q = q.Where("rule_name IN ?", s.approvableRules(u.Email))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return core.Fail(c, 500, err, "search failed")
	}
	var rows []models.PendingApproval
	if err := q.Order("id desc").Limit(limit).Offset((page - 1) * limit).Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "search failed")
	}
	for i := range rows {
		rows[i].Raw = nil
	}
	return core.Page(c, rows, int(total), page, limit)
}

func (s *Service) approvableRules(email string) []string {
	var rules []models.DlpRule
	if err := s.DB.Where("action = ?", models.DlpActionHold).Find(&rules).Error; err != nil {
		return nil
	}
	var names []string
	for _, r := range rules {
		for _, a := range splitApprovers(r.Approvers) {
			if strings.EqualFold(a, email) {
				names = append(names, r.Name)
				break
			}
		}
	}
	return names
}

func (s *Service) getApproval(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	var p models.PendingApproval
	if err := s.DB.WithContext(c.Context()).First(&p, id).Error; err != nil {
		return core.Fail(c, 404, err, "approval not found")
	}
	if !s.canApprove(c, &p) {
		return core.Fail(c, 403, nil, "approver required")
	}
	body := renderPreview(p.Raw)
	p.Raw = nil
	return c.JSON(fiber.Map{"approval": p, "preview": body})
}

func (s *Service) decide(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	var body struct {
		Decision string `json:"decision"` // approve | reject
		Reason   string `json:"reason"`
	}
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	var p models.PendingApproval
	if err := s.DB.WithContext(c.Context()).First(&p, id).Error; err != nil {
		return core.Fail(c, 404, err, "approval not found")
	}
	if !s.canApprove(c, &p) {
		return core.Fail(c, 403, nil, "approver required")
	}
	if p.Status != "pending" {
		return core.Fail(c, 409, nil, "already decided")
	}
	user := core.CurrentUser(c)
	updated, err := s.decideInternal(p.ID, body.Decision, strings.TrimSpace(body.Reason), user.Email)
	if err != nil {
		if err == errNotPending {
			return core.Fail(c, 409, err, "already decided")
		}
		if err == errBadDecision {
			return core.Fail(c, 400, err, "decision must be approve or reject")
		}
		if err == errDelivery {
			return core.Fail(c, 502, err, "delivery failed")
		}
		return core.Fail(c, 500, err, "save decision failed")
	}
	return c.JSON(updated)
}

// decideInternal applies an approve/reject decision and returns the updated
// pending row. Split out of the handler so tests exercise the same path.
func (s *Service) decideInternal(id uint, decision, reason, approver string) (*models.PendingApproval, error) {
	var p models.PendingApproval
	if err := s.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	if p.Status != "pending" {
		return nil, errNotPending
	}
	now := time.Now()
	p.Approver = approver
	p.DecisionAt = &now
	p.Reason = reason
	switch decision {
	case "approve":
		p.Status = "approved"
		if err := s.DB.Save(&p).Error; err != nil {
			return nil, err
		}
		to := splitApprovers(p.Recipients)
		if err := s.sendRaw(p.From, to, p.Raw); err != nil {
			p.Status = "delivery_failed"
			if serr := s.DB.Save(&p).Error; serr != nil {
				log.Printf("dlp save delivery_failed for pending %d: %v", p.ID, serr)
			}
			return nil, errDelivery
		}
		if err := markOutboxApproved(s.DB, p.ID); err != nil {
			log.Printf("dlp mark outbox sent for pending %d: %v", p.ID, err)
		}
	case "reject":
		p.Status = "rejected"
		// Decision and outbox flip commit together so the sender never sees a
		// held message whose approval is already final.
		if err := s.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&p).Error; err != nil {
				return err
			}
			return markOutboxRejected(tx, p.ID, p.Reason)
		}); err != nil {
			return nil, err
		}
		s.notifySender(&p, "rejected")
	default:
		return nil, errBadDecision
	}
	return &p, nil
}

var (
	errNotPending  = &errKind{"not pending"}
	errBadDecision = &errKind{"bad decision"}
	errDelivery    = &errKind{"delivery failed"}
)

type errKind struct{ msg string }

func (e *errKind) Error() string { return e.msg }
