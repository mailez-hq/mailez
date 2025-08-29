//go:build !mailez_ee

package admin

import "github.com/gofiber/fiber/v2"

// registerAI mounts nothing in the community build: the AI assistant
// configuration surface ships with the enterprise edition.
func (h *Handler) registerAI(r fiber.Router, mw fiber.Handler) {}
