// Package domain implements the domain-management domain: domains, DKIM keys,
// domain managers, alternative domains and relay hosts.
package domain

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the domain-management routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

// Register mounts the domain routes (global admin only).
func (h *Handler) Register(r fiber.Router) {
	h.registerDomains(r, h.RequireGlobalAdmin)
	h.registerDkim(r, h.RequireGlobalAdmin)
	h.registerManagers(r, h.RequireGlobalAdmin)
	h.registerAlternatives(r, h.RequireGlobalAdmin)
	h.registerRelays(r, h.RequireGlobalAdmin)
}
