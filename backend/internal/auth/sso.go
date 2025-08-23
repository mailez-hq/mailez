package auth

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/totp"
)

// RegisterSSO mounts the user-facing session endpoints.
func (m *Manager) RegisterSSO(r fiber.Router) {
	r.Post("/sso/login", m.ssoLogin)
	r.Post("/sso/login/totp", m.ssoLoginTotp)
	r.Post("/sso/logout", m.ssoLogout)
	r.Get("/sso/me", m.ssoMe)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"pw"`
}

func (m *Manager) ssoLogin(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if !m.checkLoginAttempt(c.Context(), c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many login attempts, try again later"})
	}
	sid, user, err := m.Login(c.Context(), req.Email, req.Password)
	if err != nil {
		m.loginFailed(c.Context(), req.Email)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "wrong e-mail or password"})
	}
	if user == nil {
		m.loginFailed(c.Context(), req.Email)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "wrong e-mail or password"})
	}
	m.loginSucceeded(c.Context(), user.Email)
	// A 2FA user must complete the second factor before a session is issued.
	if user.TOTPEnabled {
		pending, err := m.CreatePending2FA(c.Context(), user.Email)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "2fa setup failed"})
		}
		return c.JSON(fiber.Map{"totp_required": true, "pending_token": pending})
	}
	setSessionCookie(c, m.SessionName, sid, m.SessionTTL, m.secureCookie)
	// record the login in the audit trail
	m.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "POST", Path: "/sso/login", Status: fiber.StatusOK})
	return c.JSON(fiber.Map{"email": user.Email})
}

// ssoLoginTotp completes a login after the password step by verifying the
// authenticator code and only then creating the real session.
func (m *Manager) ssoLoginTotp(c *fiber.Ctx) error {
	var in struct {
		PendingToken string `json:"pending_token"`
		Code         string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil || in.PendingToken == "" || in.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "pending_token and code are required"})
	}
	if !m.checkLoginAttempt(c.Context(), c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many login attempts, try again later"})
	}
	email, ok := m.ConsumePending2FA(c.Context(), in.PendingToken)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "2fa session expired"})
	}
	var user models.User
	if err := m.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error; err != nil || !user.TOTPEnabled {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "2fa not configured"})
	}
	if !totp.Valid(user.TOTPSecret, in.Code, time.Now()) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid verification code"})
	}
	sid, err := m.CreateSession(c.Context(), user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "session failed"})
	}
	setSessionCookie(c, m.SessionName, sid, m.SessionTTL, m.secureCookie)
	m.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "POST", Path: "/sso/login/totp", Status: fiber.StatusOK})
	return c.JSON(fiber.Map{"email": user.Email})
}

func (m *Manager) ssoLogout(c *fiber.Ctx) error {
	sid := c.Cookies(m.SessionName)
	if sid != "" {
		_ = m.Logout(c.Context(), sid)
	}
	clearSessionCookie(c, m.SessionName)
	return c.SendStatus(fiber.StatusNoContent)
}

func (m *Manager) ssoMe(c *fiber.Ctx) error {
	sid := c.Cookies(m.SessionName)
	user, err := m.UserFromSession(c.Context(), sid)
	if err != nil || user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	manager := user.GlobalAdmin
	if !manager {
		var count int64
		if err := m.DB.WithContext(c.Context()).Table("manager").Where("user_email = ?", user.Email).Count(&count).Error; err == nil {
			manager = count > 0
		}
	}
	return c.JSON(fiber.Map{
		"email":          user.Email,
		"displayed_name": user.DisplayedName,
		"global_admin":   user.GlobalAdmin,
		"manager":        manager,
		"enabled":        user.Enabled,
	})
}

func setSessionCookie(c *fiber.Ctx, name, sid string, ttl time.Duration, secure bool) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    sid,
		Path:     "/",
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Lax",
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(c *fiber.Ctx, name string) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		MaxAge:   -1,
	})
}
