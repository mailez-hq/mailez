// White-label branding is an optional module: without it no branding
// routes mount and the login page keeps the built-in Mailez brand.
package admin

import "github.com/gofiber/fiber/v2"

func (h *Handler) registerBranding(r fiber.Router, mw fiber.Handler) {}
