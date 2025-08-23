package alias

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// MailIdentity is a From address the current user may pick when composing.
type MailIdentity struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	DkimEnabled bool   `json:"dkim_enabled"`
	Signature   string `json:"signature"`
}

// mailIdentities lists the addresses the current user may send from: their own
// address plus every non-disabled alias that delivers to them (or that they
// own as an anonymous alias). DKIM health is reported per hosting domain.
// mailIdentities lists addresses the caller may send from, with DKIM health.
// @Summary Send-as identities
// @Tags mail
// @Produce json
// @Success 200 {array} MailIdentity
// @Router /mail/identities [get]
func (h *Handler) mailIdentities(c *fiber.Ctx) error {
	user := currentUser(c)

	dkim := func(domainName string) bool {
		var d models.Domain
		if err := h.DB.Select("dkim_key").First(&d, "name = ?", domainName).Error; err != nil {
			return false
		}
		return d.DkimKey != ""
	}

	ids := []MailIdentity{{
		Email:       user.Email,
		Name:        user.DisplayedName,
		DkimEnabled: dkim(user.DomainName),
		Signature:   user.Signature,
	}}
	seen := map[string]bool{strings.ToLower(user.Email): true}

	var aliases []models.Alias
	if err := h.DB.
		Where("disabled = ?", false).
		Where("destination LIKE ? OR owner_email = ?", "%"+user.Email+"%", user.Email).
		Find(&aliases).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	for _, a := range aliases {
		key := strings.ToLower(a.Email)
		if seen[key] {
			continue
		}
		seen[key] = true
		ids = append(ids, MailIdentity{
			Email:       a.Email,
			DkimEnabled: dkim(a.DomainName),
		})
	}
	return c.JSON(ids)
}

// MaySendAs reports whether from is the user's own address or one of their
// aliases (the same rule the reference implementation applies for spoofing protection).
func MaySendAs(app *core.App, user *models.User, from string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" || strings.EqualFold(from, user.Email) {
		return true
	}
	var count int64
	if err := app.DB.Model(&models.Alias{}).
		Where("lower(email) = ? AND disabled = ? AND (destination LIKE ? OR owner_email = ?)",
			from, false, "%"+user.Email+"%", user.Email).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
