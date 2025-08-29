//go:build !mailez_ee

package user

import "github.com/gofiber/fiber/v2"

// registerSmime mounts nothing in the community build: S/MIME certificate
// management and CMS operations ship with the enterprise edition.
func (h *Handler) registerSmime(r fiber.Router) {}
