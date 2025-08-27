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
	Delegated   bool   `json:"delegated,omitempty"` // true when sending as a delegated mailbox owner
}

// mailIdentities lists the addresses the current user may send from: their own
// address, every non-disabled alias that delivers to them (or that they own as
// an anonymous alias), plus the mailboxes whose owner delegated sending to
// them. DKIM health is reported per hosting domain.
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
	// Delegated mailboxes: owners that granted this user send-as (or full
	// access, which implies send-as). The owner's display name and signature
	// travel with the identity so the compose panel looks right.
	var deps []models.MailDelegation
	if err := h.DB.Where("delegate_email = ? AND (can_send = ? OR full_access = ?)", user.Email, true, true).
		Find(&deps).Error; err == nil {
		for _, dep := range deps {
			key := strings.ToLower(dep.OwnerEmail)
			if seen[key] {
				continue
			}
			seen[key] = true
			idn := MailIdentity{Email: dep.OwnerEmail, Delegated: true, DkimEnabled: dkim(domainOf(dep.OwnerEmail))}
			var owner models.User
			if err := h.DB.First(&owner, "email = ?", dep.OwnerEmail).Error; err == nil {
				idn.Name = owner.DisplayedName
				idn.Signature = owner.Signature
			}
			ids = append(ids, idn)
		}
	}
	return c.JSON(ids)
}

// domainOf extracts the domain part of an address for DKIM health lookups.
func domainOf(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return email[i+1:]
	}
	return email
}

// aliasDeliversTo reports whether the alias destination list (a CSV column)
// or distribution-group member list contains the address exactly, or the
// user owns the alias. Substring matching would let "anna@example.com" match
// user "a@example.com".
func aliasDeliversTo(a models.Alias, email string) bool {
	if strings.EqualFold(a.OwnerEmail, email) {
		return true
	}
	for _, d := range a.Destinations() {
		if strings.EqualFold(strings.TrimSpace(d), email) {
			return true
		}
	}
	for _, m := range a.MemberList() {
		if strings.EqualFold(strings.TrimSpace(m.Email), email) {
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
		Where("destination LIKE ? OR members LIKE ? OR owner_email = ?", "%"+user.Email+"%", "%"+user.Email+"%", user.Email).
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

// MaySendAs reports whether from is the user's own address, one of their
// aliases, or a mailbox that delegated sending to them (the same rule the
// mail services apply for spoofing protection).
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
	var dep models.MailDelegation
	if err := app.DB.Where("owner_email = ? AND delegate_email = ? AND (can_send = ? OR full_access = ?)",
		from, strings.ToLower(user.Email), true, true).
		First(&dep).Error; err == nil {
		return true
	}
	return false
}
