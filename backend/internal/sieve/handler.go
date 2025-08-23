// Package sieve implements the Sieve filtering domain.
package sieve

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the Sieve routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

func (h *Handler) mailToken(c *fiber.Ctx) (string, error) { return h.App.MailToken(c) }

// Register mounts the Sieve routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerSieve(r)
}
