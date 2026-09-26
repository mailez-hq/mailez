//go:build mailez_ee

package admin

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

const maxFooterHTMLBytes = 20000

func (h *Handler) registerOrgFooter(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/org-footer", mw, h.orgFooterList)
	r.Put("/admin/org-footer", mw, h.orgFooterSave)
}

// @Summary Organization footers
// @Tags admin
// @Produce json
// @Success 200 {array} models.OrgFooter
// @Failure 403 {object} map[string]interface{}
// @Router /admin/org-footer [get]
func (h *Handler) orgFooterList(c *fiber.Ctx) error {
	rows := []models.OrgFooter{}
	if err := h.DB.Order("domain").Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "load footers failed")
	}
	return c.JSON(rows)
}

// @Summary Save organization footer
// @Tags admin
// @Accept json
// @Produce json
// @Success 200 {object} models.OrgFooter
// @Failure 400 {object} map[string]interface{}
// @Router /admin/org-footer [put]
func (h *Handler) orgFooterSave(c *fiber.Ctx) error {
	var in struct {
		Domain   string `json:"domain"`
		BodyHTML string `json:"body_html"`
		Enabled  bool   `json:"enabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	if domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "domain is required"})
	}
	var known int64
	if err := h.DB.Model(&models.Domain{}).Where("name = ?", domain).Count(&known).Error; err != nil {
		return core.Fail(c, 500, err, "domain lookup failed")
	}
	if known == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "unknown domain"})
	}
	body := mail.SanitizeSignatureHTML(in.BodyHTML)
	if len(body) > maxFooterHTMLBytes {
		return c.Status(400).JSON(fiber.Map{"error": "footer is too large"})
	}
	text := models.SignatureHTMLToText(body)
	enabled := in.Enabled && (body != "" || text != "")

	var row models.OrgFooter
	err := h.DB.Where("domain = ?", domain).First(&row).Error
	switch {
	case err == nil:
		if err := h.DB.Model(&models.OrgFooter{}).Where("id = ?", row.ID).Updates(map[string]any{
			"body_html": body,
			"body_text": text,
			"enabled":   enabled,
		}).Error; err != nil {
			return core.Fail(c, 400, err, "save footer failed")
		}
	case err == gorm.ErrRecordNotFound:
		row = models.OrgFooter{Domain: domain, BodyHTML: body, BodyText: text, Enabled: enabled}
		if err := h.DB.Create(&row).Error; err != nil {
			return core.Fail(c, 400, err, "save footer failed")
		}
	default:
		return core.Fail(c, 500, err, "load footer failed")
	}
	if err := h.DB.Where("domain = ?", domain).First(&row).Error; err != nil {
		return core.Fail(c, 500, err, "reload footer failed")
	}
	return c.JSON(row)
}
