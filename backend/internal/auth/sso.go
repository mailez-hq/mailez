package auth

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/models"
)

// RegisterSSO mounts the user-facing session endpoints.
func (m *Manager) RegisterSSO(r fiber.Router) {
	r.Post("/sso/login", m.ssoLogin)
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
	sid, user, err := m.Login(c.Context(), req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "wrong e-mail or password"})
	}
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "wrong e-mail or password"})
	}
	setSessionCookie(c, m.SessionName, sid, m.SessionTTL)
	// record the login in the audit trail
	m.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "POST", Path: "/sso/login", Status: fiber.StatusOK})
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
		if err := m.DB.Table("manager").Where("user_email = ?", user.Email).Count(&count).Error; err == nil {
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

func setSessionCookie(c *fiber.Ctx, name, sid string, ttl time.Duration) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    sid,
		Path:     "/",
		HTTPOnly: true,
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
