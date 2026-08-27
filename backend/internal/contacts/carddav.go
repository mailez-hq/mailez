package contacts

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
)

// cardDAVView is the API shape of a stored CardDAV config (password omitted).
type cardDAVView struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	HasAuth  bool   `json:"has_auth"`
}

// contactsCardDAVGet returns the user's CardDAV sync config.
// @Summary Get CardDAV config
// @Tags contacts
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /contacts/carddav [get]
func (h *Handler) contactsCardDAVGet(c *fiber.Ctx) error {
	var cfg models.CardDAVConfig
	err := h.DB.Where("user_email = ?", currentUser(c).Email).First(&cfg).Error
	if err != nil {
		return c.JSON(cardDAVView{})
	}
	return c.JSON(cardDAVView{URL: cfg.URL, Username: cfg.Username, HasAuth: cfg.PasswordEnc != ""})
}

// contactsCardDAVSet saves the user's CardDAV endpoint. An empty url clears
// the configuration.
// @Summary Save CardDAV config
// @Tags contacts
// @Accept json
// @Success 204
// @Router /contacts/carddav [put]
func (h *Handler) contactsCardDAVSet(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.URL == "" {
		if err := h.DB.Where("user_email = ?", user.Email).Delete(&models.CardDAVConfig{}).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
	var cfg models.CardDAVConfig
	if err := h.DB.Where("user_email = ?", user.Email).FirstOrCreate(&cfg, models.CardDAVConfig{
		UserEmail: user.Email,
	}).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	cfg.URL = in.URL
	cfg.Username = in.Username
	if in.Password != "" {
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.Password)
		if err != nil {
			return core.Fail(c, 500, err, "encrypt error")
		}
		cfg.PasswordEnc = enc
	}
	if err := h.DB.Save(&cfg).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// contactsCardDAVSync fetches the remote address book and imports any
// contacts that are not already present (one-way sync; local edits are kept).
// @Summary Sync CardDAV contacts
// @Tags contacts
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /contacts/carddav/sync [post]
func (h *Handler) contactsCardDAVSync(c *fiber.Ctx) error {
	user := currentUser(c)
	var cfg models.CardDAVConfig
	if err := h.DB.Where("user_email = ?", user.Email).First(&cfg).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "no CardDAV config"})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid url"})
	}
	req.Header.Set("Accept", "text/vcard")
	if cfg.Username != "" && cfg.PasswordEnc != "" {
		pw, derr := crypto.Decrypt(h.Cfg.SecretKey, cfg.PasswordEnc)
		if derr != nil {
			return core.Fail(c, 500, derr, "decrypt error")
		}
		req.SetBasicAuth(cfg.Username, pw)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": "sync failed: " + err.Error()})
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.Status(502).JSON(fiber.Map{"error": "remote returned " + resp.Status})
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return core.Fail(c, 502, err, "read error")
	}
	parsed := ParseVCard(string(body))
	added, updated, err := h.upsertContacts(user.Email, parsed)
	if err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(fiber.Map{"added": added, "updated": updated, "total": len(parsed)})
}

// upsertContacts inserts new contacts and refreshes name/comment/groups for
// existing ones, keyed case-insensitively by email.
func (h *Handler) upsertContacts(userEmail string, parsed []ParsedContact) (int, int, error) {
	var existing []models.Contact
	if err := h.DB.Where("user_email = ?", userEmail).Find(&existing).Error; err != nil {
		return 0, 0, err
	}
	byEmail := map[string]*models.Contact{}
	for i := range existing {
		byEmail[strings.ToLower(strings.TrimSpace(existing[i].Email))] = &existing[i]
	}
	added, updated := 0, 0
	for _, p := range parsed {
		email := strings.ToLower(strings.TrimSpace(p.Email))
		if email == "" {
			continue
		}
		if cur, ok := byEmail[email]; ok {
			dirty := false
			if p.Name != "" && p.Name != cur.Name {
				cur.Name = p.Name
				dirty = true
			}
			if p.Comment != "" && p.Comment != cur.Comment {
				cur.Comment = p.Comment
				dirty = true
			}
			if p.Groups != "" && !strings.Contains(cur.Groups, p.Groups) {
				cur.Groups = strings.Trim(strings.Join([]string{cur.Groups, p.Groups}, ","), ",")
				dirty = true
			}
			if p.Avatar != "" && cur.Avatar == "" {
				cur.Avatar = p.Avatar
				dirty = true
			}
			if dirty {
				if err := h.DB.Save(cur).Error; err != nil {
					return added, updated, err
				}
				updated++
			}
			continue
		}
		if err := h.DB.Create(&models.Contact{
			UserEmail: userEmail,
			DavUID:    newContactUID(),
			Name:      p.Name,
			Email:     p.Email,
			Comment:   p.Comment,
			Groups:    p.Groups,
			Avatar:    p.Avatar,
		}).Error; err != nil {
			return added, updated, err
		}
		added++
		byEmail[email] = nil
	}
	return added, updated, nil
}
