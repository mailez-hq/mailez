package compose

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// mailTemplates lists the user's compose templates.
// @Summary List compose templates
// @Tags mail
// @Produce json
// @Success 200 {array} models.Template
// @Router /mail/templates [get]
func (h *Handler) mailTemplates(c *fiber.Ctx) error {
	user := currentUser(c)
	var list []models.Template
	if err := h.DB.Where("user_email = ?", user.Email).Order("name").Find(&list).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(list)
}

// mailTemplateSave creates or updates a template.
// @Summary Save compose template
// @Tags mail
// @Accept json
// @Success 200 {object} models.Template
// @Failure 400 {object} map[string]interface{}
// @Router /mail/templates [post]
func (h *Handler) mailTemplateSave(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		ID      uint   `json:"id"`
		Name    string `json:"name"`
		Subject string `json:"subject"`
		HTML    string `json:"html"`
		Text    string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || in.Name == "" || len(in.Name) > 64 {
		return c.Status(400).JSON(fiber.Map{"error": "valid template name is required"})
	}
	tpl := models.Template{
		UserEmail: user.Email,
		Name:      in.Name,
		Subject:   in.Subject,
		HTML:      in.HTML,
		Text:      in.Text,
	}
	if in.ID != 0 {
		var existing models.Template
		if err := h.DB.Where("id = ? AND user_email = ?", in.ID, user.Email).First(&existing).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "template not found"})
		}
		tpl.ID = existing.ID
		tpl.CreatedAt = existing.CreatedAt
		if err := h.DB.Save(&tpl).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
		return c.JSON(tpl)
	}
	if err := h.DB.Create(&tpl).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(tpl)
}

// mailTemplateDelete removes a template.
// @Summary Delete compose template
// @Tags mail
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/templates/{id} [delete]
func (h *Handler) mailTemplateDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Where("id = ? AND user_email = ?", id, user.Email).Delete(&models.Template{})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "template not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
