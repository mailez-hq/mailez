//go:build !mailez_ee

// Community-edition calendar hooks: no sharing (the personal calendar sees
// only its owner's events and only the owner may edit them).
package calendar

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// canModify: only the owner may edit/delete in the community build.
func (h *Handler) canModify(ev *models.CalendarEvent, email string) bool {
	return ev.UserEmail == email
}

// visibleCalendars: the account's own calendar only.
func (h *Handler) visibleCalendars(email string) ([]string, []models.CalendarShare) {
	return []string{email}, nil
}

// registerShares mounts nothing in the community build (sharing is an
// enterprise capability).
func (h *Handler) registerShares(r fiber.Router) {}
