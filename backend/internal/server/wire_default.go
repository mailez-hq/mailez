// Base-build wiring for the optional route surfaces: no optional routes
// mount, no directory integration and no compliance workers start.
package server

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/compose"
	"mailez/backend/internal/core"
)

// registerOptionalRoutes mounts inert stand-ins for the optional read
// surfaces the clients probe unconditionally (webmail banner, delegation
// listings, S/MIME state). Each answers with its canonical empty shape, so
// the UI degrades to "no data" instead of 404 noise in the browser console.
// Calendar shares and drive share links are quota-gated instead of stubbed
// (see internal/calendar and internal/drive share_quota_*).
// Writes are deliberately not stubbed: the base UI exposes no entry points
// for them.
func registerOptionalRoutes(s *Server, app *core.App, v1, authed, stackGroup fiber.Router) {
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

// newDirectorySync returns nil in the base build: no AD/LDAP directory
// integration.
func newDirectorySync(db *gorm.DB, secretKey string) core.DirectorySync {
	return nil
}

// wireAppDirectory wires nothing in the base build.
func wireAppDirectory(s *Server, app *core.App) {}

// applyPublicBranding keeps the built-in Mailez brand: custom public
// branding is an optional module.
func applyPublicBranding(s *Server, brand *publicBranding) {}

// startComplianceWorkers starts no workers in the base build; the outbox
// runs without a content filter.
func startComplianceWorkers(db *gorm.DB, am *auth.Manager, cfg core.Config, bgCtx context.Context) compose.OutboundScanner {
	return nil
}
