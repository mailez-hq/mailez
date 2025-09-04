package admin

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// statusExtras adds no extra fields to the admin overview in the base build.
func statusExtras(db *gorm.DB) map[string]any { return nil }

// registerExtraRoutes mounts no extra admin routes in the base build.
func (h *Handler) registerExtraRoutes(r fiber.Router, mw fiber.Handler) {}
