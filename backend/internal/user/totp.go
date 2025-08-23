package user

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/totp"
)

// registerTotp mounts the self-service TOTP (2FA) endpoints. The provisioning
// secret is stored as soon as it is generated so the status call stays stable
// while the user is adding the code to their authenticator app.
func (h *Handler) registerTotp(r fiber.Router) {
	r.Get("/me/totp", h.totpStatus)
	r.Post("/me/totp/enable", h.totpEnable)
	r.Delete("/me/totp", h.totpDisable)
}

func (h *Handler) totpStatus(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.TOTPEnabled {
		return c.JSON(fiber.Map{"enabled": true})
	}
	secret := u.TOTPSecret
	if secret == "" {
		var err error
		secret, err = totp.GenerateSecret()
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		if err := h.DB.Model(u).Update("totp_secret", secret).Error; err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
	}
	return c.JSON(fiber.Map{
		"enabled": false,
		"secret":  secret,
		"otpauth": totp.URI("mailez", u.Email, secret),
	})
}

func (h *Handler) totpEnable(c *fiber.Ctx) error {
	u := currentUser(c)
	var in struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil || in.Code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code is required"})
	}
	if u.TOTPSecret == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no secret generated yet"})
	}
	if !totp.Valid(u.TOTPSecret, in.Code, time.Now()) {
		return c.Status(400).JSON(fiber.Map{"error": "invalid verification code"})
	}
	if err := h.DB.Model(u).Update("totp_enabled", true).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

func (h *Handler) totpDisable(c *fiber.Ctx) error {
	u := currentUser(c)
	var in struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil || in.Code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code is required"})
	}
	if !totp.Valid(u.TOTPSecret, in.Code, time.Now()) {
		return c.Status(400).JSON(fiber.Map{"error": "invalid verification code"})
	}
	if err := h.DB.Model(u).Updates(map[string]interface{}{"totp_enabled": false, "totp_secret": ""}).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
