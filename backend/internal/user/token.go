package user

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

// registerTokens exposes app-password management. Any signed-in user may
// manage their own tokens (the webmail DAV setup flow relies on it); global
// admins additionally see and revoke everyone's.
func (h *Handler) registerTokens(r fiber.Router) {
	r.Get("/tokens", h.listTokens)
	r.Post("/tokens", h.createToken)
	r.Delete("/tokens/:id", h.deleteToken)
}

// listTokens returns app tokens: the caller's own, or all when admin.
// @Summary List app tokens
// @Tags tokens
// @Produce json
// @Param page query int false "page number, 1-based"
// @Param limit query int false "page size"
// @Success 200 {object} models.Page
// @Failure 403 {object} models.APIError
// @Router /tokens [get]
func (h *Handler) listTokens(c *fiber.Ctx) error {
	page, limit := core.PageParams(c)
	var total int64
	q := h.DB.Model(&models.Token{})
	if !currentUser(c).GlobalAdmin {
		q = q.Where("user_email = ?", currentUser(c).Email)
	}
	if err := q.Count(&total).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	var tokens []models.Token
	offset := (page - 1) * limit
	if err := q.Order("id").Limit(limit).Offset(offset).Find(&tokens).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return core.Page(c, tokens, int(total), page, limit)
}

// createToken issues an app password. The plaintext secret is returned once;
// only its pbkdf2-sha256 hash is stored.
// createToken issues an app password; the plaintext secret is returned once.
// @Summary Create app token
// @Tags tokens
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /tokens [post]
func (h *Handler) createToken(c *fiber.Ctx) error {
	var in struct {
		Email string `json:"email"`
		IP    string `json:"ip"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	// Self-service: an empty email creates a token for the signed-in user.
	if in.Email == "" {
		in.Email = currentUser(c).Email
	}
	if !currentUser(c).GlobalAdmin && !strings.EqualFold(in.Email, currentUser(c).Email) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot create tokens for other users"})
	}
	var user models.User
	if err := h.DB.First(&user, "email = ?", in.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	secret, err := newAppToken()
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	hash, err := password.HashPBKDF2SHA256(secret)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	t := models.Token{UserEmail: in.Email, Password: hash, IP: in.IP}
	if err := h.DB.Create(&t).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.Status(201).JSON(fiber.Map{
		"id":         t.ID,
		"user_email": t.UserEmail,
		"ip":         t.IP,
		"token":      secret,
	})
}

// deleteToken revokes an application token.
// @Summary Delete app token
// @Tags tokens
// @Param id path int true "token id"
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /tokens/{id} [delete]
func (h *Handler) deleteToken(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	q := h.DB
	if !currentUser(c).GlobalAdmin {
		q = q.Where("user_email = ?", currentUser(c).Email)
	}
	res := q.Delete(&models.Token{}, "id = ?", id)
	if res.Error != nil {
		return core.Fail(c, 400, res.Error, "delete failed")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "token not found"})
	}
	// Cascade the revocation to background credentials that embed a copy of
	// this token: push subscriptions and webhooks would otherwise keep
	// logging in with the dead secret forever (the poller has no way to
	// notice). Clearing the copy forces the next subscribe/ensure to mint a
	// fresh token.
	_ = h.DB.Model(&models.PushSubscription{}).
		Where("token_id = ?", id).
		Updates(map[string]any{"token_enc": "", "token_id": 0}).Error
	_ = h.DB.Model(&models.Webhook{}).
		Where("token_id = ?", id).
		Updates(map[string]any{"token_enc": "", "token_id": 0}).Error
	return c.SendStatus(204)
}

// newAppToken generates a 32-char hex secret, matching auth.IsAppToken.
func newAppToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
