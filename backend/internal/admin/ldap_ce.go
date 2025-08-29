//go:build !mailez_ee

package admin

import "github.com/gofiber/fiber/v2"

// registerLDAP mounts nothing in the community build: the AD/LDAP
// directory integration ships with the enterprise edition.
func (h *Handler) registerLDAP(r fiber.Router, mw fiber.Handler) {}
