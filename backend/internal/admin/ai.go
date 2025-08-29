//go:build mailez_ee

package admin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/ee/ai"
)

func (h *Handler) registerAI(r fiber.Router, mw fiber.Handler) {
	r.Get("/config/ai", mw, h.listAIProviders)
	r.Post("/config/ai", mw, h.createAIProvider)
	r.Put("/config/ai/:id", mw, h.updateAIProvider)
	r.Delete("/config/ai/:id", mw, h.deleteAIProvider)
	r.Post("/config/ai/:id/test", mw, h.testAIProvider)
}

// aiConfigView is the API shape exposed to the admin UI. The API key is never
// returned; clients send a new one to rotate it (empty = keep current).
type aiConfigView struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	// HasAPIKey reports whether a key is stored (GET only).
	HasAPIKey bool `json:"has_api_key,omitempty"`
	// LastTestAt is the time of the most recent connection test; LastTestOK
	// and LastTestError describe its outcome.
	LastTestAt    *time.Time `json:"last_test_at,omitempty"`
	LastTestOK    bool       `json:"last_test_ok"`
	LastTestError string     `json:"last_test_error,omitempty"`
}

func (h *Handler) aiView(row *models.AiConfig) aiConfigView {
	name := row.Name
	if name == "" {
		// Legacy single-provider row predates provider names.
		name = "默认"
	}
	return aiConfigView{
		ID:            row.ID,
		Name:          name,
		Enabled:       row.Enabled,
		IsDefault:     row.IsDefault,
		Provider:      row.Provider,
		BaseURL:       row.BaseURL,
		Model:         row.Model,
		HasAPIKey:     row.APIKeyEnc != "",
		LastTestAt:    row.LastTestAt,
		LastTestOK:    row.LastTestOK,
		LastTestError: row.LastTestError,
	}
}

// listAIProviders returns every configured AI provider, default first.
// @Summary List AI providers
// @Tags admin
// @Success 200 {array} aiConfigView
// @Router /config/ai [get]
func (h *Handler) listAIProviders(c *fiber.Ctx) error {
	var rows []models.AiConfig
	if err := h.DB.Order("is_default DESC, id").Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	out := make([]aiConfigView, 0, len(rows))
	for i := range rows {
		out = append(out, h.aiView(&rows[i]))
	}
	return c.JSON(out)
}

type aiProviderInput struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	Enabled  *bool  `json:"enabled"`
	// IsDefault is a *bool so a false value can clear an existing default.
	IsDefault *bool `json:"is_default"`
}

func validateAIProvider(in *aiProviderInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if in.Provider != "" && in.Provider != "openai" {
		return errors.New("unsupported provider (only openai-compatible is available)")
	}
	return nil
}

// aiNameTaken reports whether another row already uses the name
// (case-insensitive).
func (h *Handler) aiNameTaken(name string, excludeID uint) (bool, error) {
	var count int64
	err := h.DB.Model(&models.AiConfig{}).
		Where("LOWER(name) = LOWER(?) AND id <> ?", name, excludeID).
		Count(&count).Error
	return count > 0, err
}

// createAIProvider adds a new provider. New providers start disabled: they
// must pass a connection test before they can be enabled.
// @Summary Create AI provider
// @Tags admin
// @Accept json
// @Produce json
// @Success 201 {object} aiConfigView
// @Router /config/ai [post]
func (h *Handler) createAIProvider(c *fiber.Ctx) error {
	var in aiProviderInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := validateAIProvider(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if taken, err := h.aiNameTaken(in.Name, 0); err != nil {
		return core.Fail(c, 500, err, "db error")
	} else if taken {
		return c.Status(409).JSON(fiber.Map{"error": "provider name already exists"})
	}
	row := models.AiConfig{
		Name:     strings.TrimSpace(in.Name),
		Provider: in.Provider,
		BaseURL:  in.BaseURL,
		Model:    in.Model,
	}
	if row.Provider == "" {
		row.Provider = "openai"
	}
	if in.APIKey != "" {
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.APIKey)
		if err != nil {
			return core.Fail(c, 500, err, "encryption failed")
		}
		row.APIKeyEnc = enc
	}
	if err := h.DB.Create(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save failed")
	}
	return c.Status(201).JSON(h.aiView(&row))
}

// updateAIProvider edits a provider. Changing any connection-affecting field
// clears the test result and disables the row (it must be re-tested). A
// provider can be enabled only after a passing test, and only an enabled
// provider can be the default.
// @Summary Update AI provider
// @Tags admin
// @Accept json
// @Produce json
// @Success 200 {object} aiConfigView
// @Router /config/ai/{id} [put]
func (h *Handler) updateAIProvider(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var in aiProviderInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Provider != "" && in.Provider != "openai" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported provider (only openai-compatible is available)"})
	}

	var row models.AiConfig
	if err := h.DB.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "provider not found"})
		}
		return core.Fail(c, 500, err, "db error")
	}
	if in.Name != "" {
		if strings.TrimSpace(in.Name) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name is required"})
		}
		if taken, err := h.aiNameTaken(in.Name, row.ID); err != nil {
			return core.Fail(c, 500, err, "db error")
		} else if taken {
			return c.Status(409).JSON(fiber.Map{"error": "provider name already exists"})
		}
	}

	// A connection-affecting change invalidates the previous test result and
	// forces the provider back to disabled until it is tested again.
	connectionChanged := false
	if in.Provider != "" && in.Provider != row.Provider {
		row.Provider = in.Provider
		connectionChanged = true
	}
	if in.BaseURL != "" && in.BaseURL != row.BaseURL {
		row.BaseURL = in.BaseURL
		connectionChanged = true
	}
	if in.Model != "" && in.Model != row.Model {
		row.Model = in.Model
		connectionChanged = true
	}
	if in.APIKey != "" {
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.APIKey)
		if err != nil {
			return core.Fail(c, 500, err, "encryption failed")
		}
		row.APIKeyEnc = enc
		connectionChanged = true
	}
	if connectionChanged {
		row.LastTestAt = nil
		row.LastTestOK = false
		row.LastTestError = ""
		row.Enabled = false
		row.IsDefault = false
	}

	if in.Name != "" {
		row.Name = strings.TrimSpace(in.Name)
	}
	if in.Enabled != nil {
		if *in.Enabled {
			if !row.LastTestOK {
				return c.Status(400).JSON(fiber.Map{"error": "请先测试连接，测试通过后才能启用"})
			}
			if row.APIKeyEnc == "" {
				return c.Status(400).JSON(fiber.Map{"error": "API 密钥不能为空"})
			}
			row.Enabled = true
		} else {
			row.Enabled = false
			row.IsDefault = false
		}
	}
	if in.IsDefault != nil && *in.IsDefault {
		if !row.Enabled || !row.LastTestOK {
			return c.Status(400).JSON(fiber.Map{"error": "只有已启用且测试通过的模型才能设为默认"})
		}
		if err := h.DB.Model(&models.AiConfig{}).Where("id <> ?", row.ID).Update("is_default", false).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
		row.IsDefault = true
	} else if in.IsDefault != nil {
		row.IsDefault = false
	}

	if err := h.DB.Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save failed")
	}
	return c.JSON(h.aiView(&row))
}

// deleteAIProvider removes a provider row.
// @Summary Delete AI provider
// @Tags admin
// @Success 204
// @Router /config/ai/{id} [delete]
func (h *Handler) deleteAIProvider(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Delete(&models.AiConfig{}, "id = ?", id)
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "provider not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// testAIProvider runs a live connection test against the provider's stored
// settings, records the outcome and time, and disables the provider when the
// test fails (only tested-pass providers may stay enabled).
// @Summary Test AI provider connection
// @Tags admin
// @Produce json
// @Success 200 {object} aiConfigView
// @Router /config/ai/{id}/test [post]
func (h *Handler) testAIProvider(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var row models.AiConfig
	if err := h.DB.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "provider not found"})
		}
		return core.Fail(c, 500, err, "db error")
	}
	if row.APIKeyEnc == "" {
		return c.Status(400).JSON(fiber.Map{"error": "API 密钥不能为空"})
	}
	key, err := crypto.Decrypt(h.Cfg.SecretKey, row.APIKeyEnc)
	if err != nil {
		return core.Fail(c, 500, err, "decryption failed")
	}
	base := row.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := row.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	p := ai.NewOpenAIProvider(base, key, model)

	now := time.Now()
	ctx, cancel := context.WithTimeout(c.UserContext(), 20*time.Second)
	defer cancel()
	if err := p.Ping(ctx); err != nil {
		row.LastTestAt = &now
		row.LastTestOK = false
		row.LastTestError = truncateAIError(err)
		row.Enabled = false
		row.IsDefault = false
		if err := h.DB.Save(&row).Error; err != nil {
			return core.Fail(c, 500, err, "save failed")
		}
		return c.JSON(h.aiView(&row))
	}

	row.LastTestAt = &now
	row.LastTestOK = true
	row.LastTestError = ""
	if err := h.DB.Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save failed")
	}
	return c.JSON(h.aiView(&row))
}

func truncateAIError(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}
