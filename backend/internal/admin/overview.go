package admin

import (
	"github.com/gofiber/fiber/v2"
	"mailez/backend/internal/core/models"
)

// registerOverview mounts the admin overview endpoint.
func (h *Handler) registerOverview(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/overview", mw, h.overview)
	r.Get("/admin/license", mw, h.licenseStatus)
}

// overview returns the operational summary for the admin console: account
// counts, compliance/DLP queues, storage usage and system identity.
// @Summary Admin overview
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /admin/overview [get]
func (h *Handler) overview(c *fiber.Ctx) error {
	db := h.DB.WithContext(c.Context())
	out := fiber.Map{
		"engine":   h.Cfg.MailEngine,
		"domain":   h.Cfg.Domain,
		"hostname": h.Cfg.Hostname,
	}
	count := func(dst *int64, model interface{}, conds ...interface{}) {
		if err := db.Model(model).Where(conds[0], conds[1:]...).Count(dst).Error; err != nil {
			*dst = -1
		}
	}
	var total, enabled, domains, aliases, ldapContacts, pendingApprovals, archived int64
	count(&total, &models.User{}, "1 = 1")
	count(&enabled, &models.User{}, "enabled = ?", true)
	count(&domains, &models.Domain{}, "1 = 1")
	count(&aliases, &models.Alias{}, "disabled = ?", false)
	count(&ldapContacts, &models.OrgContact{}, "1 = 1")
	count(&pendingApprovals, &models.PendingApproval{}, "status = ?", "pending")
	count(&archived, &models.ArchivedMessage{}, "1 = 1")
	out["users"] = total
	out["users_enabled"] = enabled
	out["domains"] = domains
	out["aliases"] = aliases
	out["org_contacts"] = ldapContacts
	out["pending_approvals"] = pendingApprovals
	out["archived_messages"] = archived

	var uploadBytes, driveBytes, driveFiles int64
	db.Model(&models.UploadedFile{}).Select("COALESCE(SUM(size),0)").Scan(&uploadBytes)
	db.Model(&models.DriveFile{}).Select("COALESCE(SUM(CASE WHEN is_dir = 0 THEN size ELSE 0 END),0)").Scan(&driveBytes)
	db.Model(&models.DriveFile{}).Where("is_dir = ?", false).Count(&driveFiles)
	out["upload_bytes"] = uploadBytes
	out["drive_bytes"] = driveBytes
	out["drive_files"] = driveFiles
	out["db_driver"] = h.Cfg.DBDriver
	out["kv_backend"] = h.Cfg.KVBackend
	out["blob_backend"] = h.Cfg.BlobBackend
	out["license"] = h.License.Status(db)
	return c.JSON(out)
}

// licenseStatus returns the current license snapshot (edition, mailbox cap,
// usage, expiry) for the admin console.
// @Summary License status
// @Tags admin
// @Produce json
// @Success 200 {object} license.Status
// @Router /admin/license [get]
func (h *Handler) licenseStatus(c *fiber.Ctx) error {
	return c.JSON(h.License.Status(h.DB.WithContext(c.Context())))
}
