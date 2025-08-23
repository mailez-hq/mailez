package push

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

const webhookTimeout = 10 * time.Second

// supportedWebhookEvents is the closed set of event names a webhook can opt
// into. Adding a new event here also enables it for existing subscriptions.
var supportedWebhookEvents = []string{"mail.received"}

// normalizeEvents de-duplicates and validates an event list, dropping anything
// outside the supported set.
func normalizeEvents(s string) string {
	seen := map[string]bool{}
	var out []string
	for _, e := range strings.Split(s, ",") {
		e = strings.TrimSpace(e)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		if !contains(supportedWebhookEvents, e) {
			continue
		}
		out = append(out, e)
	}
	return strings.Join(out, ",")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// DispatchWebhooks fans an event out to every enabled webhook of a user that
// subscribed to it. Delivery runs in the background so the poller is never
// blocked on a slow endpoint; the last status/error is recorded per webhook.
func DispatchWebhooks(db *gorm.DB, userEmail, event string, data map[string]any) {
	var hooks []models.Webhook
	if err := db.Where("user_email = ? AND enabled = ?", userEmail, true).Find(&hooks).Error; err != nil {
		log.Printf("webhook: load %s: %v", userEmail, err)
		return
	}
	for _, h := range hooks {
		if !contains(strings.Split(h.Events, ","), event) {
			continue
		}
		go deliverWebhook(db, h, event, data)
	}
}

func deliverWebhook(db *gorm.DB, h models.Webhook, event string, data map[string]any) {
	payload := map[string]any{
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		recordDelivery(db, h, 0, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mailez-webhook/1")
	if h.Secret != "" {
		mac := hmac.New(sha256.New, []byte(h.Secret))
		mac.Write(body)
		req.Header.Set("X-Mailez-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := &http.Client{Timeout: webhookTimeout}
	resp, err := client.Do(req)
	if err != nil {
		recordDelivery(db, h, 0, err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	recordDelivery(db, h, resp.StatusCode, nil)
}

func recordDelivery(db *gorm.DB, h models.Webhook, status int, err error) {
	updates := map[string]any{"last_status": status, "last_sent_at": time.Now()}
	if err != nil {
		updates["last_error"] = err.Error()
	} else {
		updates["last_error"] = ""
	}
	if uerr := db.Model(&models.Webhook{}).Where("id = ?", h.ID).Updates(updates).Error; uerr != nil {
		log.Printf("webhook: record %d: %v", h.ID, uerr)
	}
}

// Webhook CRUD ---------------------------------------------------------------

// webhookList returns every webhook configured by the current user.
// @Summary List webhooks
// @Tags webhooks
// @Produce json
// @Success 200 {array} models.Webhook
// @Router /webhooks [get]
func (h *Handler) webhookList(c *fiber.Ctx) error {
	user := currentUser(c)
	var hooks []models.Webhook
	if err := h.DB.Where("user_email = ?", user.Email).Order("id").Find(&hooks).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(hooks)
}

type webhookIn struct {
	URL     string `json:"url"`
	Secret  string `json:"secret"`
	Events  string `json:"events"`
	Enabled *bool  `json:"enabled"`
}

// webhookCreate registers a new callback endpoint for the current user.
// @Summary Create webhook
// @Tags webhooks
// @Accept json
// @Produce json
// @Success 200 {object} models.Webhook
// @Failure 400 {object} models.APIError
// @Router /webhooks [post]
func (h *Handler) webhookCreate(c *fiber.Ctx) error {
	user := currentUser(c)
	var in webhookIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	wh, msg := buildWebhook(user.Email, in)
	if msg != "" {
		return c.Status(400).JSON(fiber.Map{"error": msg})
	}
	if err := h.DB.Create(&wh).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(wh)
}

// webhookUpdate edits an existing webhook (URL, secret, events, enabled).
// @Summary Update webhook
// @Tags webhooks
// @Accept json
// @Produce json
// @Success 200 {object} models.Webhook
// @Failure 400 {object} models.APIError
// @Router /webhooks/{id} [put]
func (h *Handler) webhookUpdate(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var wh models.Webhook
	if err := h.DB.Where("id = ? AND user_email = ?", id, user.Email).First(&wh).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "webhook not found"})
	}
	var in webhookIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	// Optional fields keep their current value when omitted.
	if in.URL != "" || in.Secret != "" || in.Events != "" || in.Enabled != nil {
		if in.URL != "" {
			wh.URL = in.URL
		}
		if in.Secret != "" {
			wh.Secret = in.Secret
		}
		if in.Events != "" {
			wh.Events = in.Events
		}
		if in.Enabled != nil {
			wh.Enabled = *in.Enabled
		}
	}
	if msg := validateWebhook(wh.URL, wh.Events); msg != "" {
		return c.Status(400).JSON(fiber.Map{"error": msg})
	}
	wh.Events = normalizeEvents(wh.Events)
	if wh.Events == "" {
		return c.Status(400).JSON(fiber.Map{"error": "at least one supported event required"})
	}
	if err := h.DB.Save(&wh).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(wh)
}

// webhookDelete removes a webhook.
// @Summary Delete webhook
// @Tags webhooks
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /webhooks/{id} [delete]
func (h *Handler) webhookDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Where("id = ? AND user_email = ?", id, user.Email).Delete(&models.Webhook{})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "internal error")
	}
	return c.SendStatus(204)
}

// webhookTest fires a synchronous probe event so the user can verify the
// endpoint accepts signed deliveries.
// @Summary Test webhook
// @Tags webhooks
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /webhooks/{id}/test [post]
func (h *Handler) webhookTest(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var wh models.Webhook
	if err := h.DB.Where("id = ? AND user_email = ?", id, user.Email).First(&wh).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "webhook not found"})
	}
	status, derr := deliverWebhookSync(wh, "test", map[string]any{"message": "ping"})
	recordDelivery(h.DB, wh, status, derr)
	if derr != nil {
		return c.JSON(fiber.Map{"ok": false, "error": derr.Error()})
	}
	return c.JSON(fiber.Map{"ok": status >= 200 && status < 300, "status": status})
}

func buildWebhook(userEmail string, in webhookIn) (models.Webhook, string) {
	in.URL = strings.TrimSpace(in.URL)
	in.Events = strings.TrimSpace(in.Events)
	if msg := validateWebhook(in.URL, in.Events); msg != "" {
		return models.Webhook{}, msg
	}
	events := normalizeEvents(in.Events)
	if events == "" {
		return models.Webhook{}, "at least one supported event required"
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return models.Webhook{
		UserEmail: userEmail,
		URL:       in.URL,
		Secret:    in.Secret,
		Events:    events,
		Enabled:   enabled,
	}, ""
}

func validateWebhook(url, events string) string {
	if url == "" || !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "url must be http(s)"
	}
	return ""
}

// deliverWebhookSync is the synchronous variant used by the test endpoint.
func deliverWebhookSync(h models.Webhook, event string, data map[string]any) (int, error) {
	payload := map[string]any{
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mailez-webhook/1")
	if h.Secret != "" {
		mac := hmac.New(sha256.New, []byte(h.Secret))
		mac.Write(body)
		req.Header.Set("X-Mailez-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := &http.Client{Timeout: webhookTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
