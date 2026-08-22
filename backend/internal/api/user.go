package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailess/backend/internal/models"
	"mailess/backend/internal/password"
)

func (h *Handler) registerUsers(r fiber.Router) {
	r.Get("/users", h.listUsers)
	r.Post("/users", h.createUser)
	r.Get("/users/:email", h.getUser)
	r.Put("/users/:email", h.updateUser)
	r.Delete("/users/:email", h.deleteUser)
}

func (h *Handler) listUsers(c *fiber.Ctx) error {
	var users []models.User
	if err := h.DB.Find(&users).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(users)
}

func (h *Handler) getUser(c *fiber.Ctx) error {
	var u models.User
	if err := h.DB.First(&u, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	return c.JSON(u)
}

func (h *Handler) createUser(c *fiber.Ctx) error {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		QuotaBytes  int64  `json:"quota_bytes"`
		GlobalAdmin bool   `json:"global_admin"`
		DisplayedName string `json:"displayed_name"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Email == "" || in.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email and password are required"})
	}
	localpart, domainName, ok := strings.Cut(in.Email, "@")
	if !ok || localpart == "" || domainName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "invalid email"})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", domainName).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "domain does not exist"})
	}
	hash, err := password.Hash(in.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	u := models.User{
		Email:         in.Email,
		Localpart:     localpart,
		DomainName:    domainName,
		Password:      hash,
		GlobalAdmin:   in.GlobalAdmin,
		QuotaBytes:    in.QuotaBytes,
		DisplayedName: in.DisplayedName,
	}
	if err := h.DB.Create(&u).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(u)
}

func (h *Handler) updateUser(c *fiber.Ctx) error {
	var u models.User
	if err := h.DB.First(&u, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	var in struct {
		Password      string `json:"password"`
		QuotaBytes    int64  `json:"quota_bytes"`
		GlobalAdmin   *bool  `json:"global_admin"`
		Enabled       *bool  `json:"enabled"`
		DisplayedName string `json:"displayed_name"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Password != "" {
		hash, err := password.Hash(in.Password)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		u.Password = hash
	}
	if in.QuotaBytes != 0 {
		u.QuotaBytes = in.QuotaBytes
	}
	if in.GlobalAdmin != nil {
		u.GlobalAdmin = *in.GlobalAdmin
	}
	if in.Enabled != nil {
		u.Enabled = *in.Enabled
	}
	if in.DisplayedName != "" {
		u.DisplayedName = in.DisplayedName
	}
	if err := h.DB.Save(&u).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(u)
}

func (h *Handler) deleteUser(c *fiber.Ctx) error {
	if err := h.DB.Delete(&models.User{}, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
