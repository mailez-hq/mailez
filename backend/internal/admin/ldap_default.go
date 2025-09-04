package admin

import "github.com/gofiber/fiber/v2"

// registerLDAP mounts nothing in the base build: the AD/LDAP directory
// integration is an optional module.
func (h *Handler) registerLDAP(r fiber.Router, mw fiber.Handler) {}
