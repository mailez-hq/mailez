// Package delegation implements mailbox delegation / shared mailboxes: a
// mailbox owner grants another user the right to send as them and optionally
// to access the full mailbox. The grants back the SMTP sender-identity check,
// the IMAP/SMTP delegated login and the webmail account switcher.
package delegation

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// View is a delegation row enriched with display names for the UI.
type View struct {
	models.MailDelegation
	DelegateName string `json:"delegate_name"`
	OwnerName    string `json:"owner_name"`
}

// delegationInput is the create/update payload. FullAccess implies CanSend.
type delegationInput struct {
	DelegateEmail string `json:"delegate_email"`
	CanSend       bool   `json:"can_send"`
	FullAccess    bool   `json:"full_access"`
}

// Register mounts the delegation routes under the authenticated API.
func (h *Handler) Register(r fiber.Router) {
	r.Get("/delegations", h.delegations)
	r.Post("/delegations", h.delegationCreate)
	r.Put("/delegations/:id", h.delegationUpdate)
	r.Delete("/delegations/:id", h.delegationDelete)
}

// delegations lists the grants this user made and the grants others made to
// them.
// @Summary List mailbox delegations
// @Tags delegation
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /delegations [get]
func (h *Handler) delegations(c *fiber.Ctx) error {
	user := currentUser(c)
	var granted []models.MailDelegation
	if err := h.DB.Where("owner_email = ?", user.Email).Order("created_at").Find(&granted).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	var received []models.MailDelegation
	if err := h.DB.Where("delegate_email = ?", user.Email).Order("created_at").Find(&received).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(fiber.Map{
		"granted":  enrich(h.DB, granted),
		"received": enrich(h.DB, received),
	})
}

// delegationCreate grants a delegate access to this user's mailbox.
// @Summary Grant mailbox delegation
// @Tags delegation
// @Accept json
// @Produce json
// @Success 200 {object} delegation.View
// @Failure 400 {object} map[string]interface{}
// @Router /delegations [post]
func (h *Handler) delegationCreate(c *fiber.Ctx) error {
	user := currentUser(c)
	var in delegationInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	in.DelegateEmail = strings.ToLower(strings.TrimSpace(in.DelegateEmail))
	if !validAddress(in.DelegateEmail) {
		return c.Status(400).JSON(fiber.Map{"error": "a valid delegate email is required"})
	}
	if strings.EqualFold(in.DelegateEmail, user.Email) {
		return c.Status(400).JSON(fiber.Map{"error": "cannot delegate to yourself"})
	}
	var delegate models.User
	if err := h.DB.First(&delegate, "email = ?", in.DelegateEmail).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "delegate must be a mailbox user"})
	}
	if !delegate.Enabled {
		return c.Status(400).JSON(fiber.Map{"error": "delegate account is disabled"})
	}
	var clash int64
	if err := h.DB.Model(&models.MailDelegation{}).
		Where("owner_email = ? AND delegate_email = ?", user.Email, in.DelegateEmail).
		Count(&clash).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	if clash > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "delegation already exists"})
	}
	dep := models.MailDelegation{
		OwnerEmail:    user.Email,
		DelegateEmail: in.DelegateEmail,
		CanSend:       in.CanSend || in.FullAccess,
		FullAccess:    in.FullAccess,
	}
	if !dep.CanSend {
		return c.Status(400).JSON(fiber.Map{"error": "choose at least one permission"})
	}
	if err := h.DB.Create(&dep).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(enrich(h.DB, []models.MailDelegation{dep})[0])
}

// delegationUpdate changes the permissions of one of this user's grants.
// @Summary Update mailbox delegation
// @Tags delegation
// @Accept json
// @Produce json
// @Success 200 {object} delegation.View
// @Failure 400 {object} map[string]interface{}
// @Router /delegations/{id} [put]
func (h *Handler) delegationUpdate(c *fiber.Ctx) error {
	user := currentUser(c)
	dep, err := h.owned(c, user)
	if err != nil {
		return err
	}
	var in delegationInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	dep.CanSend = in.CanSend || in.FullAccess
	dep.FullAccess = in.FullAccess
	if !dep.CanSend {
		return c.Status(400).JSON(fiber.Map{"error": "choose at least one permission"})
	}
	if err := h.DB.Save(&dep).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(enrich(h.DB, []models.MailDelegation{*dep})[0])
}

// delegationDelete revokes one of this user's grants.
// @Summary Revoke mailbox delegation
// @Tags delegation
// @Success 204
// @Router /delegations/{id} [delete]
func (h *Handler) delegationDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	dep, err := h.owned(c, user)
	if err != nil {
		return err
	}
	if err := h.DB.Delete(&models.MailDelegation{}, dep.ID).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// owned loads a delegation row the current user created, or a 404.
func (h *Handler) owned(c *fiber.Ctx, user *models.User) (*models.MailDelegation, error) {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return nil, c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var dep models.MailDelegation
	if err := h.DB.First(&dep, "id = ? AND owner_email = ?", id, user.Email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, c.Status(404).JSON(fiber.Map{"error": "delegation not found"})
		}
		return nil, core.Fail(c, 500, err, "db error")
	}
	return &dep, nil
}

func validAddress(addr string) bool {
	if addr == "" || len(addr) > 255 || !strings.Contains(addr, "@") {
		return false
	}
	at := strings.LastIndex(addr, "@")
	return at > 0 && at < len(addr)-1 && !strings.ContainsAny(addr, " \t\r\n")
}

// enrich attaches display names to delegation rows.
func enrich(db *gorm.DB, rows []models.MailDelegation) []View {
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		v := View{MailDelegation: r}
		var u models.User
		if err := db.First(&u, "email = ?", r.DelegateEmail).Error; err == nil {
			v.DelegateName = u.DisplayedName
		}
		if err := db.First(&u, "email = ?", r.OwnerEmail).Error; err == nil {
			v.OwnerName = u.DisplayedName
		}
		out = append(out, v)
	}
	return out
}
