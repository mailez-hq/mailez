// Package mailbox implements the mailbox (read) domain: folders, unseen
// counts, message list/detail/raw, threads, search and message operations.
package mailbox

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// Handler serves the mailbox routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

// mailboxIdentity returns the email whose mailbox this request operates on:
// the signed-in user by default, or the delegated owner named by
// X-Delegate-Email when acting through a full-access grant. Label
// definitions and message flags belong to the mailbox being operated on, so
// every /mail/* handler resolves labels through this identity instead of the
// session user.
func mailboxIdentity(c *fiber.Ctx) string {
	if d := strings.TrimSpace(c.Get("X-Delegate-Email")); d != "" {
		return strings.ToLower(d)
	}
	if u := currentUser(c); u != nil {
		return strings.ToLower(u.Email)
	}
	return ""
}

// Register mounts the mailbox routes.
func (h *Handler) Register(r fiber.Router) {
	h.registerMail(r)
	h.registerAccount(r)
}
