package domain

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerAlternatives(r fiber.Router, mw fiber.Handler) {
	r.Get("/alternatives", mw, h.listAlternatives)
	r.Post("/alternatives", mw, h.createAlternative)
	r.Delete("/alternatives/:name", mw, h.deleteAlternative)
}

// listAlternatives returns alternative domain names, optionally filtered by
// ?domain=name.
// listAlternatives returns alternative domains.
// @Summary List alternatives
// @Tags domains
// @Produce json
// @Success 200 {array} models.Alternative
// @Router /alternatives [get]
func (h *Handler) listAlternatives(c *fiber.Ctx) error {
	q := h.DB.Order("name")
	if domain := c.Query("domain"); domain != "" {
		q = q.Where("domain_name = ?", domain)
	}
	var alternatives []models.Alternative
	if err := q.Find(&alternatives).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(alternatives)
}

// createAlternative adds an alternative domain.
// @Summary Create alternative
// @Tags domains
// @Accept json
// @Produce json
// @Success 201 {object} models.Alternative
// @Failure 400 {object} models.APIError
// @Router /alternatives [post]
func (h *Handler) createAlternative(c *fiber.Ctx) error {
	var in struct {
		Name       string `json:"name"`
		DomainName string `json:"domain_name"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Name == "" || in.DomainName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and domain_name are required"})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", in.DomainName).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	a := models.Alternative{Name: in.Name, DomainName: in.DomainName}
	if err := h.DB.Create(&a).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.Status(201).JSON(a)
}

// deleteAlternative removes an alternative domain.
// @Summary Delete alternative
// @Tags domains
// @Param name path string true "alternative name"
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /alternatives/{name} [delete]
func (h *Handler) deleteAlternative(c *fiber.Ctx) error {
	if err := h.DB.Delete(&models.Alternative{}, "name = ?", c.Params("name")).Error; err != nil {
		return core.Fail(c, 400, err, "delete failed")
	}
	return c.SendStatus(204)
}
