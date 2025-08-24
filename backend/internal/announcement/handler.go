// Package announcement implements the global announcement banner: a single
// admin-authored notice shown to every user in the webmail until cleared.
package announcement

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the announcement routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

// Register mounts the announcement routes. Reads are open to any signed-in
// user (the webmail banner); writes are global-admin only.
func (h *Handler) Register(r fiber.Router) {
	r.Get("/announcement", h.get)
	admin := r.Group("", h.RequireGlobalAdmin)
	admin.Put("/announcement", h.put)
	admin.Delete("/announcement", h.delete)
}

// get returns the active announcement, or 204 when none is published.
func (h *Handler) get(c *fiber.Ctx) error {
	var a models.Announcement
	if err := h.DB.Order("id desc").First(&a).Error; err != nil || !a.Enabled {
		return c.SendStatus(fiber.StatusNoContent)
	}
	return c.JSON(fiber.Map{"id": a.ID, "subject": a.Subject, "body": a.Body})
}

// put publishes (upserts) the announcement; a new subject/body replaces the
// previous single notice.
func (h *Handler) put(c *fiber.Ctx) error {
	var in struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if in.Subject == "" && in.Body == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "subject or body required"})
	}
	var a models.Announcement
	if err := h.DB.Order("id desc").First(&a).Error; err != nil {
		a = models.Announcement{}
	}
	a.Subject = in.Subject
	a.Body = in.Body
	a.Enabled = in.Enabled == nil || *in.Enabled
	if err := h.DB.Save(&a).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"id": a.ID, "subject": a.Subject, "body": a.Body, "enabled": a.Enabled})
}

// delete clears the announcement so no user sees it anymore.
func (h *Handler) delete(c *fiber.Ctx) error {
	if err := h.DB.Where("1 = 1").Delete(&models.Announcement{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
