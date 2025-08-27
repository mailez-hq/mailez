// Package contacts implements the contacts domain.
package contacts

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// Handler serves the contacts routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

// Register mounts the contacts routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerContacts(r)
}
