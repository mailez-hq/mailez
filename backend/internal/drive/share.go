// Drive share links: create/reuse/revoke a token link and the token-aware
// download authorization. Shared by both editions; the per-edition quota on
// active links lives in share_quota_ce.go / share_quota_ee.go.
package drive

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// registerShare mounts the share-link surface.
func (s *Service) registerShare(r fiber.Router) {
	r.Get("/drive/share/:id", s.share)
	r.Delete("/drive/share/:id", s.revokeShare)
}

// shareAuthorize: the owner, or anyone presenting the file's share token.
func (s *Service) shareAuthorize(row *models.DriveFile, email, token string) bool {
	if strings.EqualFold(row.UserEmail, email) {
		return true
	}
	return row.ShareToken != "" && token == row.ShareToken
}

// share creates (or reuses) a share link for a file.
// @Summary Share drive file
// @Tags drive
// @Produce json
// @Router /drive/share/{id} [get]
func (s *Service) share(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var row models.DriveFile
	if err := s.DB.First(&row, "id = ? AND user_email = ?", id, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if row.ShareToken == "" {
		// Edition quota on active links (CE: 3, EE: unlimited).
		if q := s.shareQuota(); q.maxShareTokens > 0 {
			var n int64
			if err := s.DB.Model(&models.DriveFile{}).
				Where("user_email = ? AND share_token <> '' AND trashed = ?", user.Email, false).
				Count(&n).Error; err != nil {
				return core.Fail(c, 500, err, "db error")
			}
			if n >= int64(q.maxShareTokens) {
				return c.Status(403).JSON(fiber.Map{
					"error": "share link quota reached for this edition",
					"code":  "quota_exceeded",
					"limit": q.maxShareTokens,
				})
			}
		}
		buf := make([]byte, 12)
		if _, rerr := rand.Read(buf); rerr != nil {
			return core.Fail(c, 500, rerr, "rng error")
		}
		row.ShareToken = hex.EncodeToString(buf)
		if err := s.DB.Model(&row).Update("share_token", row.ShareToken).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
	}
	return c.JSON(fiber.Map{
		"url": c.BaseURL() + "/api/v1/drive/download/" + strconv.FormatUint(uint64(row.ID), 10) + "?token=" + row.ShareToken,
	})
}

// revokeShare clears a file's share link (owner only).
// @Summary Revoke drive share link
// @Tags drive
// @Success 204
// @Router /drive/share/{id} [delete]
func (s *Service) revokeShare(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := s.DB.Model(&models.DriveFile{}).
		Where("id = ? AND user_email = ? AND share_token <> ''", id, user.Email).
		Update("share_token", "")
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
