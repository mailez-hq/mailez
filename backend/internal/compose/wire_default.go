package compose

import "github.com/gofiber/fiber/v2"

// registerOptionalRoutes mounts nothing in the base build: read receipts,
// message recall and mail merge are optional compose modules.
func (h *Handler) registerOptionalRoutes(r fiber.Router) {}
