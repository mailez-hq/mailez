package alias

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

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

	for _, a := range userAliases(h.DB, user) {
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

// aliasDeliversTo reports whether the alias destination list (a CSV column)
// contains the address exactly, or the user owns the alias. Substring
// matching would let "anna@example.com" match user "a@example.com".
func aliasDeliversTo(a models.Alias, email string) bool {
	if strings.EqualFold(a.OwnerEmail, email) {
		return true
	}
	for _, d := range a.Destinations() {
		if strings.EqualFold(strings.TrimSpace(d), email) {
			return true
		}
	}
	return false
}

// userAliases returns the enabled aliases the user may send from. The SQL
// LIKE is only a coarse prefilter; aliasDeliversTo applies the exact rule.
func userAliases(db *gorm.DB, user *models.User) []models.Alias {
	var candidates []models.Alias
	if err := db.
		Where("disabled = ?", false).
		Where("destination LIKE ? OR owner_email = ?", "%"+user.Email+"%", user.Email).
		Find(&candidates).Error; err != nil {
		return nil
	}
	var out []models.Alias
	for _, a := range candidates {
		if aliasDeliversTo(a, user.Email) {
			out = append(out, a)
		}
	}
	return out
}

// MaySendAs reports whether from is the user's own address or one of their
// aliases (the same rule the the mail stack applies for spoofing protection).
func MaySendAs(app *core.App, user *models.User, from string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" || strings.EqualFold(from, user.Email) {
		return true
	}
	for _, a := range userAliases(app.DB, user) {
		if strings.EqualFold(a.Email, from) {
			return true
		}
	}
	return false
}

