//go:build !mailez_ee

// Community-edition wiring for the enterprise seams: no enterprise routes
// mount and no compliance workers start. The public CE tree has no
// internal/ee directory at all; these stubs exist so the CE tree compiles.
package server

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/compose"
	"mailez/backend/internal/core"
)

// eeRegisterRoutes mounts inert stand-ins for the enterprise read surfaces
// the clients probe unconditionally (webmail banner, delegation listings,
// S/MIME state). Each answers with its canonical empty shape, so a
// community backend — including an EE webmail pointed at one — degrades to
// "no data" instead of 404 noise in the browser console. Calendar shares
// and drive share links are quota-gated instead of stubbed (see
// internal/calendar and internal/drive share_quota_*).
// Writes are deliberately not stubbed: community UIs expose no entry
// points, and stray EE write attempts should fail loudly.
func eeRegisterRoutes(s *Server, app *core.App, v1, authed, stackGroup fiber.Router) {
	authed.Get("/announcement", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	authed.Get("/delegations", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"granted": []any{}, "received": []any{}})
	})
	authed.Get("/me/smime", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"has_cert": false})
	})
	authed.Get("/me/smime/certs", func(c *fiber.Ctx) error {
		return c.JSON([]any{})
	})
}

// eeNewDirectorySync returns nil in the community build: no AD/LDAP
// directory integration.
func eeNewDirectorySync(db *gorm.DB, secretKey string) core.DirectorySync {
	return nil
}

// eeWireAppDirectory wires nothing in the community build.
func eeWireAppDirectory(s *Server, app *core.App) {}

// eePublicBranding keeps the built-in Mailez brand in the community build:
// white-label branding is an enterprise feature.
func eePublicBranding(s *Server, brand *publicBranding) {}

// eeStartComplianceWorkers starts no workers in the community build; the
// outbox runs without the content filter.
func eeStartComplianceWorkers(db *gorm.DB, am *auth.Manager, cfg core.Config, bgCtx context.Context) compose.OutboundScanner {
	return nil
}
