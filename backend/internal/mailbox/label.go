package mailbox

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// validLabelName enforces IMAP keyword atom rules: non-empty ASCII, no spaces
// or control characters, max 64 chars, and never a system flag (\...).
func validLabelName(name string) bool {
	if name == "" || len(name) > 64 || strings.HasPrefix(name, "\\") {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r >= 0x7f {
			return false
		}
	}
	return true
}

// mailLabels lists the user's label definitions (name + color).
// @Summary List labels
// @Tags mail
// @Produce json
// @Success 200 {array} models.Label
// @Router /mail/labels [get]
func (h *Handler) mailLabels(c *fiber.Ctx) error {
	user := currentUser(c)
	var labels []models.Label
	if err := h.DB.Where("user_email = ?", user.Email).Order("name").Find(&labels).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(labels)
}

// mailLabelSave creates or updates a label definition (color).
// @Summary Save label
// @Tags mail
// @Accept json
// @Success 200 {object} models.Label
// @Failure 400 {object} map[string]interface{}
// @Router /mail/labels [post]
func (h *Handler) mailLabelSave(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := c.BodyParser(&in); err != nil || !validLabelName(in.Name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid label name is required"})
	}
	if len(in.Color) > 16 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid color"})
	}

	res := h.DB.Model(&models.Label{}).
		Where("user_email = ? AND name = ?", user.Email, in.Name).
		Update("color", in.Color)
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	label := models.Label{UserEmail: user.Email, Name: in.Name, Color: in.Color}
	if res.RowsAffected == 0 {
		if err := h.DB.Create(&label).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
	} else if err := h.DB.Where("user_email = ? AND name = ?", user.Email, in.Name).First(&label).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(label)
}

// mailLabelRename renames a label everywhere: the definition row plus the
// IMAP keyword on every message carrying it.
// @Summary Rename label
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/labels/rename [post]
func (h *Handler) mailLabelRename(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BodyParser(&in); err != nil || !validLabelName(in.From) || !validLabelName(in.To) {
		return c.Status(400).JSON(fiber.Map{"error": "valid from and to names are required"})
	}
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	if err := h.Mail.With(d).ReplaceKeyword(d.Email, d.Token, in.From, in.To); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if err := h.DB.Model(&models.Label{}).
		Where("user_email = ? AND name = ?", user.Email, in.From).
		Update("name", in.To).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailLabelDelete removes a label definition and strips the keyword from
// every message.
// @Summary Delete label
// @Tags mail
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/labels [delete]
func (h *Handler) mailLabelDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	name := c.Query("name")
	if !validLabelName(name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid label name is required"})
	}
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	if err := h.Mail.With(d).ReplaceKeyword(d.Email, d.Token, name, ""); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if err := h.DB.Where("user_email = ? AND name = ?", user.Email, name).
		Delete(&models.Label{}).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
