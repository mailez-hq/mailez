//go:build !mailez_ee

package compose

import "github.com/gofiber/fiber/v2"

// registerEnterprise mounts nothing in the community build: read receipts,
// message recall and mail merge are enterprise compose features.
func (h *Handler) registerEnterprise(r fiber.Router) {}
