package api

import (
	"github.com/gofiber/fiber/v2"

	"mailess/backend/internal/models"
)

func (h *Handler) registerDomains(r fiber.Router) {
	r.Get("/domains", h.listDomains)
	r.Post("/domains", h.createDomain)
	r.Get("/domains/:name", h.getDomain)
	r.Put("/domains/:name", h.updateDomain)
	r.Delete("/domains/:name", h.deleteDomain)
}

func (h *Handler) listDomains(c *fiber.Ctx) error {
	var domains []models.Domain
	if err := h.DB.Find(&domains).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
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
	var in models.Domain
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	d.MaxUsers = in.MaxUsers
	d.MaxAliases = in.MaxAliases
	d.MaxQuotaBytes = in.MaxQuotaBytes
	d.SignupEnabled = in.SignupEnabled
	d.AnonmailEnabled = in.AnonmailEnabled
	d.Comment = in.Comment
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
