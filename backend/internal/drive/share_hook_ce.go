//go:build !mailez_ee

// Community-edition drive hooks: no share links (personal drive only).
package drive

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// registerShare mounts nothing in the community build.
func (s *Service) registerShare(r fiber.Router) {}

// shareAuthorize: only the owner may download in the community build.
func (s *Service) shareAuthorize(row *models.DriveFile, email, token string) bool {
	return strings.EqualFold(row.UserEmail, email)
}
