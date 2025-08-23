// Package mailbox implements the mailbox (read) domain: folders, unseen
// counts, message list/detail/raw, threads, search and message operations.
package mailbox

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the mailbox routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

// Register mounts the mailbox routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerMail(r)
}
