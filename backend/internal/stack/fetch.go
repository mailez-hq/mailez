package stack

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
)

func (h *Handler) registerFetch(r fiber.Router) {
	r.Get("/fetch", h.fetchList)
	r.Post("/fetch/:id", h.fetchDone)
}

// fetchList serves the fetchmail poller (/internal/fetch): it reads the full
// account list, with passwords decrypted for runtime use.
func (h *Handler) fetchList(c *fiber.Ctx) error {
	var fetches []models.Fetch
	if err := h.DB.Order("id").Find(&fetches).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	out := make([]fiber.Map, 0, len(fetches))
	for _, f := range fetches {
		out = append(out, fiber.Map{
			"id":         f.ID,
			"tls":        f.TLS,
			"keep":       f.Keep,
			"scan":       f.Scan,
			"invisible":  f.Invisible,
			"user_email": f.UserEmail,
			"protocol":   f.Protocol,
			"host":       f.Host,
			"port":       f.Port,
			"folders":    f.Folders,
			"username":   f.Username,
			"password":   decryptFetchPassword(h.Cfg.SecretKey, f.Password),
		})
	}
	return c.JSON(out)
}

// decryptFetchPassword unwraps the AES-GCM sealed fetch credential; values that
// do not decrypt (legacy plaintext) are passed through untouched.
func decryptFetchPassword(secret, stored string) string {
	if stored == "" {
		return ""
	}
	if plain, err := crypto.Decrypt(secret, stored); err == nil {
		return plain
	}
	return stored
}

// fetchDone is the POST /internal/fetch/<id> handler: the poller reports the
// run outcome; the JSON body is persisted as the error message together with
// the check timestamp.
func (h *Handler) fetchDone(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	var f models.Fetch
	if err := h.DB.First(&f, "id = ?", id).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	msg := string(c.Body())
	if msg != "" {
		var v any
		if json.Unmarshal([]byte(msg), &v) == nil {
			msg = fmt.Sprint(v)
		}
		if len(msg) > 1023 {
			msg = msg[:1023]
		}
	}
	now := time.Now()
	if err := h.DB.Model(&f).Updates(map[string]any{"last_check": now, "error": msg}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}
