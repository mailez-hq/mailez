// Package compose implements the composition domain: sending mail and
// persisting drafts (with recipients and attachments).
package compose

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// Handler serves the compose routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

// Register mounts the compose routes.
func (h *Handler) Register(r fiber.Router) {
	r.Post("/mail/send", h.mailSend)
	r.Post("/mail/draft", h.mailSaveDraft)
	r.Delete("/mail/outbox/:id", h.outboxCancel)
	r.Get("/mail/scheduled", h.outboxList)
	r.Get("/mail/templates", h.mailTemplates)
	r.Post("/mail/templates", h.mailTemplateSave)
	r.Delete("/mail/templates/:id", h.mailTemplateDelete)
	h.registerEnterprise(r)
}

func (h *Handler) mailToken(c *fiber.Ctx) (string, error) { return h.App.MailToken(c) }
