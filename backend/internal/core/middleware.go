package core

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// RequireAuth allows any authenticated (non-anonymous) session.
func (a *App) RequireAuth(c *fiber.Ctx) error {
	sid := c.Cookies(a.Auth.SessionName)
	user, err := a.Auth.UserFromSession(c.Context(), sid)
	if err != nil || user == nil || !user.Enabled {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
	c.Locals("user", user)
	return c.Next()
}

// RequireGlobalAdmin restricts a route to global administrators.
func (a *App) RequireGlobalAdmin(c *fiber.Ctx) error {
	if !CurrentUser(c).GlobalAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin required"})
	}
	return c.Next()
}

// RequireManager allows global admins and users holding a domain manager grant.
func (a *App) RequireManager(c *fiber.Ctx) error {
	u := CurrentUser(c)
	if u.GlobalAdmin {
		return c.Next()
	}
	var count int64
	if err := a.DB.Table("manager").Where("user_email = ?", u.Email).Count(&count).Error; err != nil {
		return Fail(c, fiber.StatusInternalServerError, err, "internal error")
	}
	if count == 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "manager required"})
	}
	return c.Next()
}

// Audit records write operations (non-GET) performed by an authenticated user.
func (a *App) Audit(c *fiber.Ctx) error {
	err := c.Next()
	user := CurrentUser(c)
	if user != nil && c.Method() != "GET" {
		entry := models.AuditLog{
			User:   user.Email,
			IP:     c.IP(),
			Method: c.Method(),
			Path:   c.Path(),
			Status: c.Response().StatusCode(),
		}
		a.DB.Create(&entry)
	}
	return err
}

// CanManageDomain reports whether the user may manage users/aliases of a domain.
func (a *App) CanManageDomain(u *models.User, domain string) bool {
	if u.GlobalAdmin {
		return true
	}
	var count int64
	if err := a.DB.Table("manager").Where("user_email = ? AND domain_name = ?", u.Email, domain).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// ManagedDomainScope returns a scoped query for the domains a manager holds.
// For global admins it returns an empty filter (no restriction).
func (a *App) ManagedDomainScope(u *models.User, q *gorm.DB) *gorm.DB {
	if u.GlobalAdmin {
		return q
	}
	var names []string
	if err := a.DB.Table("manager").Where("user_email = ?", u.Email).Pluck("domain_name", &names).Error; err != nil {
		return q.Where("1 = 0")
	}
	return q.Where("domain_name IN ?", names)
}
