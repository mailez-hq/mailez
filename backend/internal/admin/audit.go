package admin

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerAudit(r fiber.Router, mw fiber.Handler) {
	r.Get("/audit", mw, h.listAudit)
}

// listAudit returns the most recent audit entries, newest first.
// listAudit returns recent admin audit entries.
// @Summary Audit log
// @Tags admin
// @Produce json
// @Success 200 {array} models.AuditLog
// @Failure 403 {object} models.APIError
// @Router /audit [get]
func (h *Handler) listAudit(c *fiber.Ctx) error {
	var entries []models.AuditLog
	if err := h.DB.Order("id desc").Limit(200).Find(&entries).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(entries)
}
