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

// eeRegisterRoutes mounts nothing in the community build.
func eeRegisterRoutes(s *Server, app *core.App, v1, authed, stackGroup fiber.Router) {}

// eeNewDirectorySync returns nil in the community build: no AD/LDAP
// directory integration.
func eeNewDirectorySync(db *gorm.DB, secretKey string) core.DirectorySync {
	return nil
}

// eeWireAppDirectory wires nothing in the community build.
func eeWireAppDirectory(s *Server, app *core.App) {}

// eeStartComplianceWorkers starts no workers in the community build; the
// outbox runs without the content filter.
func eeStartComplianceWorkers(db *gorm.DB, am *auth.Manager, cfg core.Config, bgCtx context.Context) compose.OutboundScanner {
	return nil
}
