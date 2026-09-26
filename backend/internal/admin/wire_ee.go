//go:build mailez_ee

package admin

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/ee/license"
)

// statusExtras adds the license snapshot to the admin overview.
func statusExtras(db *gorm.DB) map[string]any {
	return map[string]any{"license": license.FromEnv().Status(db)}
}

// registerExtraRoutes mounts /admin/license and the enterprise-only traffic
// report for the admin console.
func (h *Handler) registerExtraRoutes(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/license", mw, func(c *fiber.Ctx) error {
		return c.JSON(license.FromEnv().Status(h.DB.WithContext(c.Context())))
	})
	h.registerOrgFooter(r, mw)
	h.registerTraffic(r, mw)
}
