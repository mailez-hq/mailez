package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/models"
)

func (h *Handler) registerAliases(r fiber.Router, mw fiber.Handler) {
	r.Get("/aliases", mw, h.listAliases)
	r.Post("/aliases", mw, h.createAlias)
	r.Get("/aliases/:email", mw, h.getAlias)
	r.Put("/aliases/:email", mw, h.updateAlias)
	r.Delete("/aliases/:email", mw, h.deleteAlias)
}

func (h *Handler) listAliases(c *fiber.Ctx) error {
	q := h.DB
	if u := currentUser(c); !u.GlobalAdmin {
		q = h.managedDomainScope(u, q)
	}
	var aliases []models.Alias
	if err := q.Find(&aliases).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(aliases)
}

func (h *Handler) getAlias(c *fiber.Ctx) error {
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.canManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	return c.JSON(a)
}

func (h *Handler) createAlias(c *fiber.Ctx) error {
	var in struct {
		Email       string `json:"email"`
		Destination string `json:"destination"`
		Wildcard    bool   `json:"wildcard"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	localpart, domainName, ok := strings.Cut(in.Email, "@")
	if !ok || localpart == "" || domainName == "" || in.Destination == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email and destination are required"})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", domainName).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "domain does not exist"})
	}
	if !h.canManageDomain(currentUser(c), domainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	a := models.Alias{
		Email:       in.Email,
		Localpart:   localpart,
		DomainName:  domainName,
		Destination: in.Destination,
		Wildcard:    in.Wildcard,
	}
	if err := h.DB.Create(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(a)
}

func (h *Handler) updateAlias(c *fiber.Ctx) error {
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.canManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	var in struct {
		Destination string `json:"destination"`
		Wildcard    *bool  `json:"wildcard"`
		Disabled    *bool  `json:"disabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Destination != "" {
		a.Destination = in.Destination
	}
	if in.Wildcard != nil {
		a.Wildcard = *in.Wildcard
	}
	if in.Disabled != nil {
		a.Disabled = *in.Disabled
	}
	if err := h.DB.Save(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(a)
}

func (h *Handler) deleteAlias(c *fiber.Ctx) error {
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.canManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	if err := h.DB.Delete(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
