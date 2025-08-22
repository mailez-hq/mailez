package api

import (
	"net/url"

	"github.com/gofiber/fiber/v2"

	"mailess/backend/internal/models"
)

func (h *Handler) registerManagers(r fiber.Router, mw fiber.Handler) {
	r.Get("/domains/:name/managers", mw, h.listDomainManagers)
	r.Post("/domains/:name/managers", mw, h.addDomainManager)
	r.Delete("/domains/:name/managers/:email", mw, h.removeDomainManager)
}

// listDomainManagers returns the users that administrate a domain.
func (h *Handler) listDomainManagers(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	var managers []models.User
	if err := h.DB.Model(&d).Association("Managers").Find(&managers); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(managers)
}

func (h *Handler) addDomainManager(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var user models.User
	if err := h.DB.First(&user, "email = ?", in.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if err := h.DB.Model(&d).Association("Managers").Append(&user); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

func (h *Handler) removeDomainManager(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	email, _ := url.QueryUnescape(c.Params("email"))
	var user models.User
	if err := h.DB.First(&user, "email = ?", email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if err := h.DB.Model(&d).Association("Managers").Delete(&user); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
