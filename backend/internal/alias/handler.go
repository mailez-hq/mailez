// Package alias implements the alias domain: aliases, anonymous aliases
// (masked email) and send-as identities.
package alias

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// Handler serves the alias-domain routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

// Register mounts the alias routes: anonymous aliases are self-service,
// regular aliases are manager-scoped, identities are self-service.
func (h *Handler) Register(r fiber.Router) {
	h.registerAnonmail(r)
	h.registerAliases(r, h.RequireManager)
	r.Get("/mail/identities", h.mailIdentities)
}
