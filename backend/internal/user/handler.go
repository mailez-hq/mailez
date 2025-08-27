// Package user implements the account domain: user provisioning (manager/admin),
// self-service profile, password, 2FA, PGP keys and app tokens.
package user

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// Handler serves the user domain routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

// RegisterPublic mounts endpoints reachable without a session (signup).
func RegisterPublic(r fiber.Router, app *core.App) {
	New(app).registerSignup(r)
}

// Register mounts the authenticated user-domain routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerMe(r)
	h.registerTotp(r)
	h.registerPGP(r)
	h.registerSmime(r)
	h.registerTokens(r)
	h.registerUsers(r, h.RequireManager)
}
