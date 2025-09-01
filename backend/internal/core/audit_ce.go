//go:build !mailez_ee

package core

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// auditEnrich is the community stub of the enterprise audit seam: CE keeps
// the raw method/path record without semantic enrichment.
func auditEnrich(*fiber.Ctx, *models.AuditLog) {}
