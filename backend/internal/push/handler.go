package push

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/password"
)

// Handler serves the push domain routes.
type Handler struct {
	*core.App
	Events *Hub
}

var currentUser = core.CurrentUser

func newAppToken() (string, error) { return core.NewAppToken() }

// RegisterAPI mounts the push routes. hub backs the SSE mailbox-change
// stream; pass NewHub() when the stream is enabled.
func RegisterAPI(r fiber.Router, app *core.App, hub *Hub) {
	h := &Handler{app, hub}
	h.registerPush(r)
}

func (h *Handler) registerPush(r fiber.Router) {
	r.Get("/push/vapid", h.pushVapid)
	r.Post("/push/subscribe", h.pushSubscribe)
	r.Delete("/push/subscribe", h.pushUnsubscribe)
	r.Get("/events", h.mailEvents)
	r.Get("/webhooks", h.webhookList)
	r.Post("/webhooks", h.webhookCreate)
	r.Put("/webhooks/:id", h.webhookUpdate)
	r.Delete("/webhooks/:id", h.webhookDelete)
	r.Post("/webhooks/:id/test", h.webhookTest)
}

// pushVapid returns the application-server key the browser must pass to
// pushManager.subscribe().
// pushVapid returns the application-server VAPID public key.
// @Summary VAPID public key
// @Tags push
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /push/vapid [get]
func (h *Handler) pushVapid(c *fiber.Ctx) error {
	key, err := EnsureVAPID(h.DB)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(fiber.Map{"public_key": key.PublicKey})
}

type pushKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// pushSubscribe stores (or refreshes) a Web Push endpoint for the current
// user and provisions the encrypted notifier token on first use.
// pushSubscribe stores a Web Push endpoint for the current user.
// @Summary Subscribe to push
// @Tags push
// @Accept json
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /push/subscribe [post]
func (h *Handler) pushSubscribe(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		Endpoint string   `json:"endpoint"`
		Keys     pushKeys `json:"keys"`
	}
	if err := c.BodyParser(&in); err != nil || in.Endpoint == "" || in.Keys.P256dh == "" || in.Keys.Auth == "" {
		return c.Status(400).JSON(fiber.Map{"error": "endpoint and keys are required"})
	}

	var tokenEnc string
	var tokenID uint
	var tokenHash string
	needToken := true
	var existing models.PushSubscription
	if err := h.DB.Where("user_email = ?", user.Email).First(&existing).Error; err == nil {
		tokenEnc, tokenID = existing.TokenEnc, existing.TokenID
		needToken = false
	} else {
		secret, err := newAppToken()
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		hash, err := password.HashPBKDF2SHA256(secret)
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, secret)
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		tokenHash, tokenEnc = hash, enc
	}
	// Provisioning the notifier token and storing the subscription happen in
	// one transaction so a failure never leaves credentials without a
	// subscription row.
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if needToken {
			t := models.Token{UserEmail: user.Email, Password: tokenHash, IP: "push-notifier"}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
			tokenID = t.ID
		}
		var count int64
		if err := tx.Model(&models.PushSubscription{}).
			Where("user_email = ? AND endpoint = ?", user.Email, in.Endpoint).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return tx.Model(&models.PushSubscription{}).
				Where("user_email = ? AND endpoint = ?", user.Email, in.Endpoint).
				Updates(map[string]any{
					"p256dh": in.Keys.P256dh, "auth": in.Keys.Auth,
					"token_enc": tokenEnc, "token_id": tokenID,
				}).Error
		}
		sub := models.PushSubscription{
			UserEmail: user.Email,
			Endpoint:  in.Endpoint,
			P256DH:    in.Keys.P256dh,
			Auth:      in.Keys.Auth,
			TokenEnc:  tokenEnc,
			TokenID:   tokenID,
		}
		return tx.Create(&sub).Error
	}); err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.SendStatus(204)
}

// pushUnsubscribe removes an endpoint and drops the notifier token once the
// user's last subscription is gone.
// pushUnsubscribe removes a Web Push endpoint.
// @Summary Unsubscribe from push
// @Tags push
// @Accept json
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /push/subscribe [delete]
func (h *Handler) pushUnsubscribe(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	if err := c.BodyParser(&in); err != nil || in.Endpoint == "" {
		return c.Status(400).JSON(fiber.Map{"error": "endpoint is required"})
	}
	var subs []models.PushSubscription
	if err := h.DB.Where("user_email = ?", user.Email).Find(&subs).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	removed := false
	// Subscription removal and notifier-token cleanup commit together, so no
	// orphaned subscription or token is left behind on partial failure.
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, s := range subs {
			if s.Endpoint != in.Endpoint {
				continue
			}
			if err := tx.Delete(&models.PushSubscription{}, "id = ?", s.ID).Error; err != nil {
				return err
			}
			removed = true
		}
		if removed && len(subs) == 1 {
			return tx.Delete(&models.Token{}, "id = ?", subs[0].TokenID).Error
		}
		return nil
	}); err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.SendStatus(204)
}
