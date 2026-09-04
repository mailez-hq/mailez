package admin

import "github.com/gofiber/fiber/v2"

// registerAuditExtra is the default implementation of the audit seam: the
// CSV export endpoint ships in an optional module.
func (h *Handler) registerAuditExtra(fiber.Router, fiber.Handler) {}
