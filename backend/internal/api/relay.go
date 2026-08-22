package api

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/models"
)

func (h *Handler) registerRelays(r fiber.Router, mw fiber.Handler) {
	r.Get("/relays", mw, h.listRelays)
	r.Post("/relays", mw, h.createRelay)
	r.Put("/relays/:name", mw, h.updateRelay)
	r.Delete("/relays/:name", mw, h.deleteRelay)
}

func (h *Handler) listRelays(c *fiber.Ctx) error {
	var relays []models.Relay
	if err := h.DB.Order("name").Find(&relays).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(relays)
}

func (h *Handler) createRelay(c *fiber.Ctx) error {
	var in struct {
		Name string `json:"name"`
		SMTP string `json:"smtp"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	r := models.Relay{Name: in.Name, SMTP: in.SMTP}
	if err := h.DB.Create(&r).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(r)
}

func (h *Handler) updateRelay(c *fiber.Ctx) error {
	var r models.Relay
	if err := h.DB.First(&r, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "relay not found"})
	}
	var in struct {
		SMTP string `json:"smtp"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	r.SMTP = in.SMTP
	if err := h.DB.Save(&r).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(r)
}

func (h *Handler) deleteRelay(c *fiber.Ctx) error {
	if err := h.DB.Delete(&models.Relay{}, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
