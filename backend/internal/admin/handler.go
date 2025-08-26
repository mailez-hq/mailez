// Package admin implements the global-administration domain: audit trail and
// configuration export/import.
package admin

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the admin routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

// Register mounts the admin routes (global admin only).
func (h *Handler) Register(r fiber.Router) {
	h.registerAudit(r, h.RequireGlobalAdmin)
	h.registerConfig(r, h.RequireGlobalAdmin)
	h.registerAI(r, h.RequireGlobalAdmin)
}
