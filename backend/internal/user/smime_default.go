package user

import "github.com/gofiber/fiber/v2"

// registerSmime mounts nothing in the base build: S/MIME certificate
// management and CMS operations are an optional module.
func (h *Handler) registerSmime(r fiber.Router) {}
