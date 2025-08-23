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

func (h *Handler) listDomains(c *fiber.Ctx) error {
	var domains []models.Domain
	if err := h.DB.Find(&domains).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(domains)
}

func (h *Handler) getDomain(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	return c.JSON(d)
}

func (h *Handler) createDomain(c *fiber.Ctx) error {
	var d models.Domain
	if err := c.BodyParser(&d); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if d.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if err := h.DB.Create(&d).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(d)
}

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
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(d)
}

func (h *Handler) deleteDomain(c *fiber.Ctx) error {
	if err := h.DB.Delete(&models.Domain{}, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
