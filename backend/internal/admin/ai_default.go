package admin

import "github.com/gofiber/fiber/v2"

// registerAI mounts nothing in the base build: the AI assistant
// configuration surface is an optional module.
func (h *Handler) registerAI(r fiber.Router, mw fiber.Handler) {}
