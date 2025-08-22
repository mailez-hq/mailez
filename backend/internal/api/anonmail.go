package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/models"
)

func (h *Handler) registerAnonmail(r fiber.Router) {
	r.Get("/anon-aliases", h.listAnonAliases)
	r.Get("/anon-aliases/domains", h.anonmailDomains)
	r.Post("/anon-aliases", h.createAnonAlias)
	r.Delete("/anon-aliases/:email", h.deleteAnonAlias)
}

// anonmailDomains lists domains the current user may create anonymous aliases
// on (anonmail enabled plus an access grant).
func (h *Handler) anonmailDomains(c *fiber.Ctx) error {
	user := currentUser(c)
	var domains []models.Domain
	if err := h.DB.Where("anonmail_enabled = ?", true).Order("name").Find(&domains).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var out []string
	for _, d := range domains {
		if h.canUseAnonmailDomain(user, d.Name) {
			out = append(out, d.Name)
		}
	}
	return c.JSON(out)
}

// listAnonAliases returns the anonymous aliases owned by the current user
// (or all of them for a global admin).
func (h *Handler) listAnonAliases(c *fiber.Ctx) error {
	user := currentUser(c)
	q := h.DB.Order("email")
	if !user.GlobalAdmin {
		q = q.Where("owner_email = ?", user.Email)
	}
	var aliases []models.Alias
	if err := q.Find(&aliases).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(aliases)
}

// createAnonAlias issues a random alias that forwards to the current user,
// e.g. "my-shop.3fa8c2d1@example.com". Only usable on anonmail-enabled domains
// the user has access to (their own domain, an explicit DomainAccess grant,
// or as a global admin).
func (h *Handler) createAnonAlias(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		Domain      string `json:"domain"`
		DisplayName string `json:"display_name"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Domain == "" || in.DisplayName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "domain and display_name are required"})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", in.Domain).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	if !domain.AnonmailEnabled && !user.GlobalAdmin {
		return c.Status(403).JSON(fiber.Map{"error": "anonymous mail is disabled for this domain"})
	}
	if !h.canUseAnonmailDomain(user, in.Domain) {
		return c.Status(403).JSON(fiber.Map{"error": "no access to this domain"})
	}
	suffix, err := randomHex(4)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	localpart := sanitizeLocalpart(in.DisplayName) + "." + suffix
	a := models.Alias{
		Email:       localpart + "@" + in.Domain,
		Localpart:   localpart,
		DomainName:  in.Domain,
		Destination: user.Email,
		OwnerEmail:  user.Email,
		Hostname:    in.Domain,
	}
	if err := h.DB.Create(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(a)
}

// deleteAnonAlias removes an anonymous alias owned by the current user.
func (h *Handler) deleteAnonAlias(c *fiber.Ctx) error {
	user := currentUser(c)
	email, _ := url.QueryUnescape(c.Params("email"))
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if a.OwnerEmail == "" {
		return c.Status(400).JSON(fiber.Map{"error": "not an anonymous alias"})
	}
	if a.OwnerEmail != user.Email && !user.GlobalAdmin {
		return c.Status(403).JSON(fiber.Map{"error": "not your alias"})
	}
	if err := h.DB.Delete(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

// canUseAnonmailDomain grants access if the user is a global admin, belongs to
// the domain, or holds an explicit DomainAccess record.
func (h *Handler) canUseAnonmailDomain(u *models.User, domain string) bool {
	if u.GlobalAdmin || u.DomainName == domain {
		return true
	}
	var grant models.DomainAccess
	return h.DB.First(&grant, "user_email = ? AND domain_name = ?", u.Email, domain).Error == nil
}

// sanitizeLocalpart keeps only safe characters for a display name localpart.
func sanitizeLocalpart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "anon"
	}
	return strings.Trim(out, ".")
}

func randomHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
