package stack

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

func (h *Handler) registerRspamd(r fiber.Router) {
	r.Get("/rspamd/vault/v1/dkim/:domain", h.rspamdDkimKey)
	r.Get("/rspamd/local_domains", h.rspamdLocalDomains)
}

// rspamdDkimKey mirrors the the mail stack's /internal/rspamd/vault/v1/dkim/<domain>.
// Always 200: selectors stay empty unless the queried domain (or an
// alternative's canonical domain) carries a DKIM key. Alternatives inherit the
// key and advertise the queried (alternative) name.
func (h *Handler) rspamdDkimKey(c *fiber.Ctx) error {
	domainName, _ := url.PathUnescape(c.Params("domain"))
	key := ""
	if domainName != "" {
		var d models.Domain
		if err := h.DB.Where("name IN ?", domainCandidates(domainName)).First(&d).Error; err == nil {
			key = d.DkimKey
		} else {
			var alt models.Alternative
			if err := h.DB.Where("name IN ?", domainCandidates(domainName)).First(&alt).Error; err == nil {
				var d2 models.Domain
				if err := h.DB.Where("name IN ?", domainCandidates(alt.DomainName)).First(&d2).Error; err == nil {
					key = d2.DkimKey
				}
			}
		}
	}
	selectors := []fiber.Map{}
	if key != "" {
		selectors = append(selectors, fiber.Map{
			"domain":   asciiDomain(strings.ToLower(domainName)),
			"key":      key,
			"selector": h.Cfg.DkimSelector,
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"selectors": selectors}})
}

// rspamdLocalDomains mirrors the the mail stack: a plain-text, newline-separated list of
// served domains and alternatives (not JSON).
func (h *Handler) rspamdLocalDomains(c *fiber.Ctx) error {
	var names []string
	if err := h.DB.Model(&models.Domain{}).Pluck("name", &names).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	var alts []string
	if err := h.DB.Model(&models.Alternative{}).Pluck("name", &alts).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	names = append(names, alts...)
	return c.Type("text/plain").SendString(strings.Join(names, "\n"))
}
