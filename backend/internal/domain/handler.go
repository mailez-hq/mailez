// Package domain implements the domain-management domain: domains, DKIM keys,
// domain managers, alternative domains and relay hosts.
package domain

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Handler serves the domain-management routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

// Register mounts the domain routes: reads are open to a domain's managers,
// mutations stay global-admin only.
func (h *Handler) Register(r fiber.Router) {
	h.registerDomains(r, h.RequireManager)
	h.registerDkim(r, h.RequireManager)
	h.registerDnsWizard(r, h.RequireManager)
	h.registerManagers(r, h.RequireGlobalAdmin)
	h.registerAlternatives(r, h.RequireGlobalAdmin)
	h.registerRelays(r, h.RequireGlobalAdmin)
}

// scopeDomains narrows a domain query to the domains the caller manages.
// ManagedDomainScope filters on domain_name; the domain table's column is name.
func (h *Handler) scopeDomains(u *models.User, q *gorm.DB) *gorm.DB {
	if u.GlobalAdmin {
		return q
	}
	var names []string
	if err := h.DB.Table("manager").Where("user_email = ?", u.Email).Pluck("domain_name", &names).Error; err != nil {
		return q.Where("1 = 0")
	}
	return q.Where("name IN ?", names)
}
