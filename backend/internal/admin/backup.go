// Admin API for the scheduled encrypted backups: configuration and run
// history, a manual trigger, and per-run verification that re-reads the
// archive from the target and decrypts it.
package admin

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/backup"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// BackupStatus is the /admin/backup payload.
type BackupStatus struct {
	Configured bool               `json:"configured"`
	Target     string             `json:"target"`
	KeySet     bool               `json:"key_set"`
	Keep       int                `json:"keep"`
	Hour       int                `json:"hour"`
	Running    bool               `json:"running"`
	Runs       []models.BackupRun `json:"runs"`
}

func (h *Handler) registerBackup(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/backup", mw, h.backupStatus)
	r.Post("/admin/backup/run", mw, h.backupRunNow)
	r.Post("/admin/backup/verify/:id", mw, h.backupVerify)
}

// backupStatus returns the backup configuration and recent runs.
// @Summary Backup status and history
// @Tags admin
// @Produce json
// @Success 200 {object} BackupStatus
// @Failure 403 {object} models.APIError
// @Router /admin/backup [get]
func (h *Handler) backupStatus(c *fiber.Ctx) error {
	e := backup.New(h.DB, h.Cfg)
	var runs []models.BackupRun
	if err := h.DB.Order("id desc").Limit(20).Find(&runs).Error; err != nil {
		return core.Fail(c, fiber.StatusInternalServerError, err, "internal error")
	}
	var unfinished int64
	h.DB.Model(&models.BackupRun{}).
		Where("finished_at IS NULL AND started_at > ?", time.Now().Add(-6*time.Hour)).Count(&unfinished)
	return c.JSON(BackupStatus{
		Configured: e.Configured(),
		Target:     h.Cfg.BackupTarget,
		KeySet:     h.Cfg.BackupKey != "",
		Keep:       h.Cfg.BackupKeep,
		Hour:       h.Cfg.BackupHour,
		Running:    unfinished > 0,
		Runs:       runs,
	})
}

// backupRunNow triggers one archive asynchronously; progress shows up in
// the run history.
// @Summary Trigger a backup
// @Tags admin
// @Success 202
// @Failure 400 {object} models.APIError
// @Failure 403 {object} models.APIError
// @Router /admin/backup/run [post]
func (h *Handler) backupRunNow(c *fiber.Ctx) error {
	e := backup.New(h.DB, h.Cfg)
	if !e.Configured() {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{Error: "backup target not configured"})
	}
	go func() {
		if err := e.RunNow(context.Background()); err != nil {
			log.Printf("manual backup: %v", err)
		}
	}()
	return c.SendStatus(fiber.StatusAccepted)
}

// backupVerify re-reads an archived run from the target and decrypts it.
// @Summary Verify a backup archive
// @Tags admin
// @Param id path int true "backup run id"
// @Success 200 {object} map[string]any "entries"
// @Failure 403 {object} models.APIError
// @Failure 404 {object} models.APIError
// @Router /admin/backup/verify/{id} [post]
func (h *Handler) backupVerify(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{Error: "invalid id"})
	}
	entries, err := backup.New(h.DB, h.Cfg).Verify(c.Context(), uint(id))
	if err != nil {
		return core.Fail(c, fiber.StatusUnprocessableEntity, err, "verification failed")
	}
	return c.JSON(fiber.Map{"ok": true, "entries": entries})
}
