// Package sieve implements the Sieve filtering domain.
package sieve

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// Handler serves the Sieve routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

func (h *Handler) mailToken(c *fiber.Ctx) (string, error) { return h.App.MailToken(c) }

// Register mounts the Sieve routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerSieve(r)
}
