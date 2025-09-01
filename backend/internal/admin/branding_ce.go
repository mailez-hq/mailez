//go:build !mailez_ee

// Community stub: white-label branding is enterprise-only, so no branding
// routes mount on a CE backend and the login page keeps the built-in
// Mailez brand.
package admin

import "github.com/gofiber/fiber/v2"

func (h *Handler) registerBranding(r fiber.Router, mw fiber.Handler) {}
