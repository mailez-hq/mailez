package push

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/password"
)

// Handler serves the push domain routes.
type Handler struct {
	*core.App
}

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

func newAppToken() (string, error) { return core.NewAppToken() }

// RegisterAPI mounts the push routes.
func RegisterAPI(r fiber.Router, app *core.App) {
	h := &Handler{app}
	h.registerPush(r)
}

func (h *Handler) registerPush(r fiber.Router) {
	r.Get("/push/vapid", h.pushVapid)
	r.Post("/push/subscribe", h.pushSubscribe)
	r.Delete("/push/subscribe", h.pushUnsubscribe)
}

// pushVapid returns the application-server key the browser must pass to
// pushManager.subscribe().
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
	var existing models.PushSubscription
	if err := h.DB.Where("user_email = ?", user.Email).First(&existing).Error; err == nil {
		tokenEnc, tokenID = existing.TokenEnc, existing.TokenID
	} else {
		secret, err := newAppToken()
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		hash, err := password.HashPBKDF2SHA256(secret)
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		t := models.Token{UserEmail: user.Email, Password: hash, IP: "push-notifier"}
		if err := h.DB.Create(&t).Error; err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		tokenID = t.ID
		tokenEnc, err = crypto.Encrypt(h.Cfg.SecretKey, secret)
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
	}

	var count int64
	h.DB.Model(&models.PushSubscription{}).
		Where("user_email = ? AND endpoint = ?", user.Email, in.Endpoint).
		Count(&count)
	if count > 0 {
		if err := h.DB.Model(&models.PushSubscription{}).
			Where("user_email = ? AND endpoint = ?", user.Email, in.Endpoint).
			Updates(map[string]any{
				"p256dh": in.Keys.P256dh, "auth": in.Keys.Auth,
				"token_enc": tokenEnc, "token_id": tokenID,
			}).Error; err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
	} else {
		sub := models.PushSubscription{
			UserEmail: user.Email,
			Endpoint:  in.Endpoint,
			P256DH:    in.Keys.P256dh,
			Auth:      in.Keys.Auth,
			TokenEnc:  tokenEnc,
			TokenID:   tokenID,
		}
		if err := h.DB.Create(&sub).Error; err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
	}
	return c.SendStatus(204)
}

// pushUnsubscribe removes an endpoint and drops the notifier token once the
// user's last subscription is gone.
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
	for _, s := range subs {
		if s.Endpoint == in.Endpoint {
			if err := h.DB.Delete(&models.PushSubscription{}, "id = ?", s.ID).Error; err != nil {
				return core.Fail(c, 500, err, "internal error")
			}
			removed = true
		}
	}
	if removed && len(subs) == 1 {
		_ = h.DB.Delete(&models.Token{}, "id = ?", subs[0].TokenID)
	}
	return c.SendStatus(204)
}
