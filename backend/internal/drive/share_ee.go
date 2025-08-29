//go:build mailez_ee

// Drive share links (team capability): create/reuse a token link and the
// token-aware download authorization. Enterprise only.
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
