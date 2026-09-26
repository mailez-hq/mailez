package user

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

const maxSignatureHTMLBytes = 20000

func (h *Handler) registerSignatures(r fiber.Router) {
	r.Get("/me/signatures", h.signatureList)
	r.Post("/me/signatures", h.signatureCreate)
	r.Put("/me/signatures/:id", h.signatureUpdate)
	r.Delete("/me/signatures/:id", h.signatureDelete)
	r.Put("/me/signatures/:id/default", h.signatureSetDefault)
}

type signatureIn struct {
	Name            string `json:"name"`
	IdentityEmail   string `json:"identity_email"`
	BodyHTML        string `json:"body_html"`
	DefaultForNew   bool   `json:"default_for_new"`
	DefaultForReply bool   `json:"default_for_reply"`
}

func (in *signatureIn) normalise() (string, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Signature"
	}
	if utf8.RuneCountInString(name) > 120 {
		return "", "", errors.New("signature name is too long")
	}
	body := mail.SanitizeSignatureHTML(in.BodyHTML)
	if len(body) > maxSignatureHTMLBytes {
		return "", "", errors.New("signature is too large")
	}
	if models.SignatureHTMLToText(body) == "" {
		return "", "", errors.New("signature body is empty")
	}
	return name, body, nil
}

// @Summary List signatures
// @Tags me
// @Produce json
// @Success 200 {array} models.Signature
// @Failure 401 {object} map[string]interface{}
// @Router /me/signatures [get]
func (h *Handler) signatureList(c *fiber.Ctx) error {
	u := currentUser(c)
	rows := []models.Signature{}
	if err := h.DB.Where("user_email = ?", u.Email).Order("id").Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "load signatures failed")
	}
	return c.JSON(rows)
}

// @Summary Create signature
// @Tags me
// @Accept json
// @Produce json
// @Success 200 {object} models.Signature
// @Failure 400 {object} map[string]interface{}
// @Router /me/signatures [post]
func (h *Handler) signatureCreate(c *fiber.Ctx) error {
	u := currentUser(c)
	var in signatureIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	name, body, err := in.normalise()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	sig := models.Signature{
		UserEmail:       u.Email,
		IdentityEmail:   strings.ToLower(strings.TrimSpace(in.IdentityEmail)),
		Name:            name,
		BodyHTML:        body,
		BodyText:        models.SignatureHTMLToText(body),
		DefaultForNew:   in.DefaultForNew,
		DefaultForReply: in.DefaultForReply,
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.Signature{}).Where("user_email = ?", u.Email).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			sig.DefaultForNew = true
			sig.DefaultForReply = true
		}
		if sig.DefaultForNew {
			if err := clearSignatureDefault(tx, u.Email, sig.IdentityEmail, "default_for_new"); err != nil {
				return err
			}
		}
		if sig.DefaultForReply {
			if err := clearSignatureDefault(tx, u.Email, sig.IdentityEmail, "default_for_reply"); err != nil {
				return err
			}
		}
		return tx.Create(&sig).Error
	}); err != nil {
		return core.Fail(c, 400, err, "create signature failed")
	}
	return c.JSON(sig)
}

// @Summary Update signature
// @Tags me
// @Accept json
// @Produce json
// @Success 200 {object} models.Signature
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /me/signatures/{id} [put]
func (h *Handler) signatureUpdate(c *fiber.Ctx) error {
	u := currentUser(c)
	sig, ok := h.ownedSignature(c, u.Email)
	if !ok {
		return nil
	}
	var in signatureIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	name, body, err := in.normalise()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	identity := strings.ToLower(strings.TrimSpace(in.IdentityEmail))
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if in.DefaultForNew {
			if err := clearSignatureDefault(tx, u.Email, identity, "default_for_new"); err != nil {
				return err
			}
		}
		if in.DefaultForReply {
			if err := clearSignatureDefault(tx, u.Email, identity, "default_for_reply"); err != nil {
				return err
			}
		}
		return tx.Model(&models.Signature{}).Where("id = ?", sig.ID).Updates(map[string]any{
			"name":              name,
			"identity_email":    identity,
			"body_html":         body,
			"body_text":         models.SignatureHTMLToText(body),
			"default_for_new":   in.DefaultForNew,
			"default_for_reply": in.DefaultForReply,
		}).Error
	}); err != nil {
		return core.Fail(c, 400, err, "update signature failed")
	}
	updated, ok := h.loadSignature(c, sig.ID)
	if !ok {
		return nil
	}
	return c.JSON(updated)
}

// @Summary Delete signature
// @Tags me
// @Success 204
// @Failure 404 {object} map[string]interface{}
// @Router /me/signatures/{id} [delete]
func (h *Handler) signatureDelete(c *fiber.Ctx) error {
	u := currentUser(c)
	sig, ok := h.ownedSignature(c, u.Email)
	if !ok {
		return nil
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.Signature{}, sig.ID).Error; err != nil {
			return err
		}
		if !sig.DefaultForNew && !sig.DefaultForReply {
			return nil
		}
		var next models.Signature
		err := tx.Where("user_email = ? AND identity_email = ?", u.Email, sig.IdentityEmail).
			Order("id").First(&next).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]any{}
		if sig.DefaultForNew {
			updates["default_for_new"] = true
		}
		if sig.DefaultForReply {
			updates["default_for_reply"] = true
		}
		return tx.Model(&models.Signature{}).Where("id = ?", next.ID).Updates(updates).Error
	}); err != nil {
		return core.Fail(c, 400, err, "delete signature failed")
	}
	return c.SendStatus(204)
}

// @Summary Set signature default
// @Tags me
// @Accept json
// @Produce json
// @Success 200 {object} models.Signature
// @Failure 400 {object} map[string]interface{}
// @Router /me/signatures/{id}/default [put]
func (h *Handler) signatureSetDefault(c *fiber.Ctx) error {
	u := currentUser(c)
	sig, ok := h.ownedSignature(c, u.Email)
	if !ok {
		return nil
	}
	var in struct {
		Kind    string `json:"kind"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var column string
	switch in.Kind {
	case "new":
		column = "default_for_new"
	case "reply":
		column = "default_for_reply"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "kind must be new or reply"})
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if in.Enabled {
			if err := clearSignatureDefault(tx, u.Email, sig.IdentityEmail, column); err != nil {
				return err
			}
		}
		return tx.Model(&models.Signature{}).Where("id = ?", sig.ID).Update(column, in.Enabled).Error
	}); err != nil {
		return core.Fail(c, 400, err, "update signature failed")
	}
	updated, ok := h.loadSignature(c, sig.ID)
	if !ok {
		return nil
	}
	return c.JSON(updated)
}

func (h *Handler) saveLegacySignature(u *models.User, text string) error {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	return h.DB.Transaction(func(tx *gorm.DB) error {
		var sig models.Signature
		err := tx.Where("user_email = ? AND identity_email = '' AND default_for_new = ?", u.Email, true).
			Order("id").First(&sig).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Where("user_email = ? AND identity_email = ''", u.Email).Order("id").First(&sig).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if text == "" {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return tx.Model(&models.Signature{}).Where("id = ?", sig.ID).
				Updates(map[string]any{"default_for_new": false, "default_for_reply": false}).Error
		}
		body := models.PlainTextToSignatureHTML(text)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&models.Signature{
				UserEmail:       u.Email,
				Name:            "Default",
				BodyHTML:        body,
				BodyText:        text,
				DefaultForNew:   true,
				DefaultForReply: true,
			}).Error
		}
		return tx.Model(&models.Signature{}).Where("id = ?", sig.ID).Updates(map[string]any{
			"body_html":         body,
			"body_text":         text,
			"default_for_new":   true,
			"default_for_reply": true,
		}).Error
	})
}

func (h *Handler) legacySignatureText(email string) string {
	sig := models.DefaultSignature(h.DB, email, email, false)
	if sig == nil {
		sig = models.DefaultSignature(h.DB, email, "", false)
	}
	if sig == nil {
		return ""
	}
	return sig.BodyText
}

func clearSignatureDefault(tx *gorm.DB, userEmail, identity, column string) error {
	return tx.Model(&models.Signature{}).
		Where("user_email = ? AND identity_email = ?", userEmail, identity).
		Update(column, false).Error
}

func (h *Handler) ownedSignature(c *fiber.Ctx, email string) (*models.Signature, bool) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		_ = c.Status(400).JSON(fiber.Map{"error": "invalid signature id"})
		return nil, false
	}
	sig, ok := h.loadSignature(c, uint(id), email)
	if !ok {
		return nil, false
	}
	return sig, true
}

func (h *Handler) loadSignature(c *fiber.Ctx, id uint, owner ...string) (*models.Signature, bool) {
	q := h.DB.Where("id = ?", id)
	if len(owner) > 0 {
		q = q.Where("user_email = ?", owner[0])
	}
	var sig models.Signature
	if err := q.First(&sig).Error; err != nil {
		_ = c.Status(404).JSON(fiber.Map{"error": "signature not found"})
		return nil, false
	}
	return &sig, true
}
