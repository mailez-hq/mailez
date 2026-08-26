package admin

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
)

func (h *Handler) registerAI(r fiber.Router, mw fiber.Handler) {
	r.Get("/config/ai", mw, h.getAIConfig)
	r.Put("/config/ai", mw, h.putAIConfig)
}

// aiConfigView is the API shape exposed to the admin UI. The API key is never
// returned; clients send a new one to rotate it (empty = keep current).
type aiConfigView struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	// HasAPIKey reports whether a key is stored (GET only).
	HasAPIKey bool `json:"has_api_key,omitempty"`
}

// getAIConfig returns the current AI provider settings.
// @Summary Get AI configuration
// @Tags admin
// @Success 200 {object} aiConfigView
// @Router /config/ai [get]
func (h *Handler) getAIConfig(c *fiber.Ctx) error {
	var row models.AiConfig
	if err := h.DB.First(&row).Error; err != nil {
		// No row yet: report the disabled/empty default.
		return c.JSON(aiConfigView{Provider: "openai"})
	}
	return c.JSON(aiConfigView{
		Enabled:   row.Enabled,
		Provider:  row.Provider,
		BaseURL:   row.BaseURL,
		Model:     row.Model,
		HasAPIKey: row.APIKeyEnc != "",
	})
}

// putAIConfig upserts the AI provider settings. An empty api_key keeps the
// stored key; a non-empty one rotates it.
// @Summary Update AI configuration
// @Tags admin
// @Accept json
// @Success 200 {object} aiConfigView
// @Router /config/ai [put]
func (h *Handler) putAIConfig(c *fiber.Ctx) error {
	var in struct {
		Enabled  *bool  `json:"enabled"`
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Provider != "" && in.Provider != "openai" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported provider (only openai-compatible is available)"})
	}

	var row models.AiConfig
	if err := h.DB.First(&row).Error; err != nil {
		row = models.AiConfig{ID: 1}
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.Provider != "" {
		row.Provider = in.Provider
	}
	row.BaseURL = in.BaseURL
	row.Model = in.Model
	if in.APIKey != "" {
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.APIKey)
		if err != nil {
			return core.Fail(c, 500, err, "encryption failed")
		}
		row.APIKeyEnc = enc
	}
	if err := h.DB.Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save failed")
	}
	return c.JSON(aiConfigView{
		Enabled:   row.Enabled,
		Provider:  row.Provider,
		BaseURL:   row.BaseURL,
		Model:     row.Model,
		HasAPIKey: row.APIKeyEnc != "",
	})
}
