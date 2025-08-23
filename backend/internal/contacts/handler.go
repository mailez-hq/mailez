// Package contacts implements the contacts domain.
package contacts

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the contacts routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

// Register mounts the contacts routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerContacts(r)
}
