package admin

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerAudit(r fiber.Router, mw fiber.Handler) {
	r.Get("/audit", mw, h.listAudit)
}

// listAudit returns recent admin audit entries, newest first, paginated.
// @Summary Audit log
// @Tags admin
// @Produce json
// @Param page query int false "page number, 1-based"
// @Param limit query int false "page size"
// @Success 200 {object} models.Page
// @Failure 403 {object} models.APIError
// @Router /audit [get]
func (h *Handler) listAudit(c *fiber.Ctx) error {
	page, limit := core.PageParams(c)
	var total int64
	if err := h.DB.Model(&models.AuditLog{}).Count(&total).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	var entries []models.AuditLog
	offset := (page - 1) * limit
	if err := h.DB.Order("id desc").Limit(limit).Offset(offset).Find(&entries).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return core.Page(c, entries, int(total), page, limit)
}
