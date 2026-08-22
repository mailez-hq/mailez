package api

import (
	"github.com/gofiber/fiber/v2"

	"mailess/backend/internal/models"
)

func (h *Handler) registerAudit(r fiber.Router, mw fiber.Handler) {
	r.Get("/audit", mw, h.listAudit)
}

// auditMiddleware records write operations (non-GET) performed by an
// authenticated user for the audit trail.
func (h *Handler) auditMiddleware(c *fiber.Ctx) error {
	err := c.Next()
	user := currentUser(c)
	if user != nil && c.Method() != "GET" {
		entry := models.AuditLog{
			User:   user.Email,
			IP:     c.IP(),
			Method: c.Method(),
			Path:   c.Path(),
			Status: c.Response().StatusCode(),
		}
		h.DB.Create(&entry)
	}
	return err
}

// listAudit returns the most recent audit entries, newest first.
func (h *Handler) listAudit(c *fiber.Ctx) error {
	var entries []models.AuditLog
	if err := h.DB.Order("id desc").Limit(200).Find(&entries).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(entries)
}
