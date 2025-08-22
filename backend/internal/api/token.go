package api

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/models"
	"mailez/backend/internal/password"
)

func (h *Handler) registerTokens(r fiber.Router, mw fiber.Handler) {
	r.Get("/tokens", mw, h.listTokens)
	r.Post("/tokens", mw, h.createToken)
	r.Delete("/tokens/:id", mw, h.deleteToken)
}

func (h *Handler) listTokens(c *fiber.Ctx) error {
	var tokens []models.Token
	if err := h.DB.Order("id").Find(&tokens).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokens)
}

// createToken issues an app password. The plaintext secret is returned once;
// only its pbkdf2-sha256 hash is stored.
func (h *Handler) createToken(c *fiber.Ctx) error {
	var in struct {
		Email string `json:"email"`
		IP    string `json:"ip"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email is required"})
	}
	var user models.User
	if err := h.DB.First(&user, "email = ?", in.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	secret, err := newAppToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	hash, err := password.HashPBKDF2SHA256(secret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	t := models.Token{UserEmail: in.Email, Password: hash, IP: in.IP}
	if err := h.DB.Create(&t).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{
		"id":        t.ID,
		"user_email": t.UserEmail,
		"ip":        t.IP,
		"token":     secret,
	})
}

func (h *Handler) deleteToken(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.DB.Delete(&models.Token{}, "id = ?", id).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

// newAppToken generates a 32-char hex secret, matching auth.IsAppToken.
func newAppToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
