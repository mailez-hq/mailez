package admin

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerBranding(r fiber.Router, mw fiber.Handler) {
	r.Get("/branding", mw, h.getBranding)
	r.Put("/branding", mw, h.putBranding)
}

// brandingView is the API shape for the admin console. All fields are
// optional: an empty value means "use the built-in Mailez brand".
type brandingView struct {
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Tagline   string `json:"tagline"`
	Feature1  string `json:"feature1"`
	Feature2  string `json:"feature2"`
	Feature3  string `json:"feature3"`
	LogoURL   string `json:"logo_url"`
	HeroURL   string `json:"hero_url"`
	Copyright string `json:"copyright"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func viewBranding(row *models.BrandingConfig) brandingView {
	out := brandingView{
		Title:     row.Title,
		Subtitle:  row.Subtitle,
		Tagline:   row.Tagline,
		Feature1:  row.Feature1,
		Feature2:  row.Feature2,
		Feature3:  row.Feature3,
		LogoURL:   row.LogoURL,
		HeroURL:   row.HeroURL,
		Copyright: row.Copyright,
	}
	if !row.UpdatedAt.IsZero() {
		out.UpdatedAt = row.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}

// getBranding returns the current login-page branding settings.
// @Summary Get branding configuration
// @Tags admin
// @Success 200 {object} brandingView
// @Router /branding [get]
func (h *Handler) getBranding(c *fiber.Ctx) error {
	var row models.BrandingConfig
	if err := h.DB.First(&row).Error; err != nil {
		return c.JSON(brandingView{})
	}
	return c.JSON(viewBranding(&row))
}

// putBranding saves the login-page branding settings.
// @Summary Update branding configuration
// @Tags admin
// @Accept json
// @Success 200 {object} brandingView
// @Router /branding [put]
func (h *Handler) putBranding(c *fiber.Ctx) error {
	var in struct {
		Title     string `json:"title"`
		Subtitle  string `json:"subtitle"`
		Tagline   string `json:"tagline"`
		Feature1  string `json:"feature1"`
		Feature2  string `json:"feature2"`
		Feature3  string `json:"feature3"`
		LogoURL   string `json:"logo_url"`
		HeroURL   string `json:"hero_url"`
		Copyright string `json:"copyright"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	var row models.BrandingConfig
	if err := h.DB.First(&row).Error; err != nil {
		row = models.BrandingConfig{ID: 1}
	}
	row.Title = strings.TrimSpace(in.Title)
	row.Subtitle = strings.TrimSpace(in.Subtitle)
	row.Tagline = strings.TrimSpace(in.Tagline)
	row.Feature1 = strings.TrimSpace(in.Feature1)
	row.Feature2 = strings.TrimSpace(in.Feature2)
	row.Feature3 = strings.TrimSpace(in.Feature3)
	row.LogoURL = strings.TrimSpace(in.LogoURL)
	row.HeroURL = strings.TrimSpace(in.HeroURL)
	row.Copyright = strings.TrimSpace(in.Copyright)
	if err := h.DB.Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save failed")
	}
	return c.JSON(viewBranding(&row))
}
