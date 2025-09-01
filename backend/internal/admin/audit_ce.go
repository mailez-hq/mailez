//go:build !mailez_ee

package admin

import "github.com/gofiber/fiber/v2"

// registerAuditExtra is the community stub of the audit seam: the CSV export
// endpoint is enterprise-only.
func (h *Handler) registerAuditExtra(fiber.Router, fiber.Handler) {}
