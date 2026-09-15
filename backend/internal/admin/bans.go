// Admin API over the ban records: list active bans, lift one early. The
// endpoints read the table directly rather than going through the engine,
// so history stays manageable even with the engine disabled. Lifts are
// DELETE mutations, so the audit middleware records them like every other
// admin write.
package admin

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerBans(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/bans", mw, h.listBans)
	r.Delete("/admin/bans/:id", mw, h.liftBan)
}

// listBans returns the currently active IP bans.
// @Summary Active IP bans
// @Tags admin
// @Produce json
// @Success 200 {array} models.BanRecord
// @Failure 403 {object} models.APIError
// @Router /admin/bans [get]
func (h *Handler) listBans(c *fiber.Ctx) error {
	var records []models.BanRecord
	if err := h.DB.WithContext(c.Context()).
		Where("lifted_at IS NULL AND until > ?", time.Now()).
		Order("until desc").Limit(200).Find(&records).Error; err != nil {
		return core.Fail(c, fiber.StatusInternalServerError, err, "internal error")
	}
	return c.JSON(records)
}

// liftBan ends a ban early; the row stays as history.
// @Summary Lift an IP ban
// @Tags admin
// @Param id path int true "ban record id"
// @Success 204
// @Failure 400 {object} models.APIError
// @Failure 403 {object} models.APIError
// @Failure 404 {object} models.APIError
// @Router /admin/bans/{id} [delete]
func (h *Handler) liftBan(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{Error: "invalid id"})
	}
	var rec models.BanRecord
	if err := h.DB.First(&rec, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.APIError{Error: "ban not found"})
	}
	now := time.Now()
	if err := h.DB.Model(&rec).Updates(map[string]any{"lifted_at": now, "until": now}).Error; err != nil {
		return core.Fail(c, fiber.StatusInternalServerError, err, "internal error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
