package core

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// auditEnrich enriches nothing in the base build: the audit trail keeps
// the raw method/path record without semantic enrichment.
func auditEnrich(*fiber.Ctx, *models.AuditLog) {}
