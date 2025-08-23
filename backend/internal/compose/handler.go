// Package compose implements the composition domain: sending mail and
// persisting drafts (with recipients and attachments).
package compose

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the compose routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

// Register mounts the compose routes.
func (h *Handler) Register(r fiber.Router) {
	r.Post("/mail/send", h.mailSend)
	r.Post("/mail/draft", h.mailSaveDraft)
}

func (h *Handler) mailToken(c *fiber.Ctx) (string, error) { return h.App.MailToken(c) }
