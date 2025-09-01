package auth

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/webauthn"
)

// RegisterPasskey mounts the passwordless passkey (WebAuthn) login routes.
// A no-op until WebAuthn is wired on the manager; the routes answer 404 so
// clients can feature-detect.
func (m *Manager) RegisterPasskey(r fiber.Router) {
	r.Post("/sso/passkey/login/begin", m.passkeyLoginBegin)
	r.Post("/sso/passkey/login/finish", m.passkeyLoginFinish)
}

// passkeyLoginBegin returns the assertion options for the account's
// registered credentials.
// @Summary Start passkey login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body object true "email"
// @Success 200 {object} map[string]any "options"
// @Failure 404 {object} models.APIError "no passkey for account / feature off"
// @Router /sso/passkey/login/begin [post]
func (m *Manager) passkeyLoginBegin(c *fiber.Ctx) error {
	if m.WebAuthn == nil {
		return c.Status(fiber.StatusNotFound).JSON(models.APIError{Error: "passkey sign-in is not enabled", Code: "passkey_disabled"})
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.Email) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !m.checkLoginAttempt(c.Context(), c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Error: "too many login attempts, try again later", Code: "rate_limited"})
	}
	options, err := m.WebAuthn.BeginLogin(email)
	if err != nil {
		if errors.Is(err, webauthn.ErrUnavailable) {
			return c.Status(fiber.StatusNotFound).JSON(models.APIError{Error: "no passkey registered for this account", Code: "passkey_absent"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "passkey ceremony failed"})
	}
	return c.JSON(fiber.Map{"options": options})
}

// passkeyLoginFinish verifies the assertion and issues the session.
// @Summary Complete passkey login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body object true "email + authenticator assertion"
// @Success 200 {object} map[string]string "email"
// @Failure 401 {object} models.APIError
// @Router /sso/passkey/login/finish [post]
func (m *Manager) passkeyLoginFinish(c *fiber.Ctx) error {
	if m.WebAuthn == nil {
		return c.Status(fiber.StatusNotFound).JSON(models.APIError{Error: "passkey sign-in is not enabled", Code: "passkey_disabled"})
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.Email) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !m.checkLoginAttempt(c.Context(), c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Error: "too many login attempts, try again later", Code: "rate_limited"})
	}
	if err := m.WebAuthn.FinishLogin(email, c.Body()); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "passkey verification failed"})
	}
	var user models.User
	if err := m.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error; err != nil || !user.Enabled {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "account unavailable"})
	}
	m.loginSucceeded(c.Context(), user.Email)
	if m.NotifyLogin != nil && m.rememberLoginIP(c.Context(), user.Email, c.IP()) {
		m.NotifyLogin(user.Email, c.IP(), c.Get("User-Agent"))
	}
	sid, err := m.CreateSession(c.Context(), user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "session failed"})
	}
	setSessionCookie(c, m.SessionName, sid, m.SessionTTL, m.secureCookie)
	m.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "POST", Path: "/sso/passkey/login/finish", Status: fiber.StatusOK, Action: "login.passkey"})
	return c.JSON(fiber.Map{"email": user.Email})
}
