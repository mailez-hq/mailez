package domain

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerDomains(r fiber.Router, mw fiber.Handler) {
	r.Get("/domains", mw, h.listDomains)
	r.Post("/domains", mw, h.createDomain)
	r.Get("/domains/:name", mw, h.getDomain)
	r.Put("/domains/:name", mw, h.updateDomain)
	r.Delete("/domains/:name", mw, h.deleteDomain)
}

// listDomains returns all domains (admin), paginated.
// @Summary List domains
// @Tags domains
// @Produce json
// @Param page query int false "page number, 1-based"
// @Param limit query int false "page size"
// @Success 200 {object} models.Page
// @Failure 403 {object} models.APIError
// @Router /domains [get]
func (h *Handler) listDomains(c *fiber.Ctx) error {
	page, limit := core.PageParams(c)
	var total int64
	if err := h.DB.Model(&models.Domain{}).Count(&total).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	var domains []models.Domain
	offset := (page - 1) * limit
	if err := h.DB.Order("name").Limit(limit).Offset(offset).Find(&domains).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return core.Page(c, domains, int(total), page, limit)
}

// getDomain returns one domain.
// @Summary Get domain
// @Tags domains
// @Produce json
// @Param name path string true "domain name"
// @Success 200 {object} models.Domain
// @Router /domains/{name} [get]
func (h *Handler) getDomain(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	return c.JSON(d)
}

// createDomain adds a domain.
// @Summary Create domain
// @Tags domains
// @Accept json
// @Produce json
// @Success 201 {object} models.Domain
// @Failure 400 {object} models.APIError
// @Router /domains [post]
func (h *Handler) createDomain(c *fiber.Ctx) error {
	var d models.Domain
	if err := c.BodyParser(&d); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if d.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if err := h.DB.Create(&d).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.Status(201).JSON(d)
}

// updateDomain updates a domain.
// @Summary Update domain
// @Tags domains
// @Accept json
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /domains/{name} [put]
func (h *Handler) updateDomain(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	var in struct {
		MaxUsers        *int    `json:"max_users"`
		MaxAliases      *int    `json:"max_aliases"`
		MaxQuotaBytes   *int64  `json:"max_quota_bytes"`
		SignupEnabled   *bool   `json:"signup_enabled"`
		AnonmailEnabled *bool   `json:"anonmail_enabled"`
		Comment         *string `json:"comment"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.MaxUsers != nil {
		d.MaxUsers = *in.MaxUsers
	}
	if in.MaxAliases != nil {
		d.MaxAliases = *in.MaxAliases
	}
	if in.MaxQuotaBytes != nil {
		d.MaxQuotaBytes = *in.MaxQuotaBytes
	}
	if in.SignupEnabled != nil {
		d.SignupEnabled = *in.SignupEnabled
	}
	if in.AnonmailEnabled != nil {
		d.AnonmailEnabled = *in.AnonmailEnabled
	}
	if in.Comment != nil {
		d.Comment = *in.Comment
	}
	if err := h.DB.Save(&d).Error; err != nil {
		return core.Fail(c, 400, err, "update failed")
	}
	return c.JSON(d)
}

// deleteDomain removes a domain.
// @Summary Delete domain
// @Tags domains
// @Param name path string true "domain name"
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /domains/{name} [delete]
func (h *Handler) deleteDomain(c *fiber.Ctx) error {
	if err := h.DB.Delete(&models.Domain{}, "name = ?", c.Params("name")).Error; err != nil {
		return core.Fail(c, 400, err, "delete failed")
	}
	return c.SendStatus(204)
}
