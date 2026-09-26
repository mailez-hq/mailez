package stack

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/mailflow"
)

func (h *Handler) registerOrgFooter(r fiber.Router) {
	r.Get("/org-footer", h.orgFooter)
}

// @Summary Organization footer lookup
// @Tags stack
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /stack/org-footer [get]
func (h *Handler) orgFooter(c *fiber.Ctx) error {
	domain := strings.ToLower(strings.TrimSpace(c.Query("domain")))
	if domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "domain is required"})
	}
	row := mailflow.OrgFooterFor(h.DB, domain)
	if row == nil {
		return c.JSON(fiber.Map{"domain": domain, "enabled": false})
	}
	return c.JSON(fiber.Map{
		"domain":    row.Domain,
		"enabled":   row.Enabled,
		"body_html": row.BodyHTML,
		"body_text": row.BodyText,
	})
}
