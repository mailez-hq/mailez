package user

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/pgp"
)

// registerPGP mounts the built-in OpenPGP surface: key management for the
// current user, a public-key lookup directory, and encrypt/decrypt/sign/verify
// operations for the webmail composer and reading pane.
func (h *Handler) registerPGP(r fiber.Router) {
	r.Get("/me/pgp", h.pgpStatus)
	r.Post("/me/pgp/generate", h.pgpGenerate)
	r.Delete("/me/pgp", h.pgpDelete)
	r.Get("/pgp/key", h.pgpLookup)
	r.Get("/me/pgp/keys", h.pgpKeys)
	r.Post("/me/pgp/keys", h.pgpImportKey)
	r.Delete("/me/pgp/keys/:id", h.pgpDeleteKey)
	r.Post("/mail/pgp/encrypt", h.pgpEncrypt)
	r.Post("/mail/pgp/decrypt", h.pgpDecrypt)
	r.Post("/mail/pgp/sign", h.pgpSign)
	r.Post("/mail/pgp/verify", h.pgpVerify)
}

// pgpStatus reports whether the current user has a key and, if so, its public
// half and fingerprint. The private key never leaves the server.
// pgpStatus reports whether the user has a PGP key.
// @Summary PGP status
// @Tags me
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /me/pgp [get]
func (h *Handler) pgpStatus(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.PGPPublicKey == "" {
		return c.JSON(fiber.Map{"has_key": false})
	}
	return c.JSON(fiber.Map{
		"has_key":     true,
		"public_key":  u.PGPPublicKey,
		"fingerprint": u.PGPFingerprint,
	})
}

// pgpGenerate creates a fresh key pair for the current user, storing the
// private key encrypted at rest with the server secret.
// pgpGenerate creates a PGP key pair for the user.
// @Summary Generate PGP key
// @Tags me
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.APIError
// @Router /me/pgp/generate [post]
func (h *Handler) pgpGenerate(c *fiber.Ctx) error {
	u := currentUser(c)
	pub, priv, err := pgp.GenerateKeyPair(u.Email)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	fp, err := pgp.Fingerprint(pub)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	encPriv, err := crypto.Encrypt(h.Cfg.SecretKey, priv)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	if err := h.DB.Model(u).Updates(map[string]interface{}{
		"pgp_public_key":  pub,
		"pgp_private_key": encPriv,
		"pgp_fingerprint": fp,
	}).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.JSON(fiber.Map{"public_key": pub, "fingerprint": fp})
}

// pgpDelete removes the current user's key pair.
// pgpDelete removes the user's PGP key.
// @Summary Delete PGP key
// @Tags me
// @Success 204
// @Router /me/pgp [delete]
func (h *Handler) pgpDelete(c *fiber.Ctx) error {
	if err := h.DB.Model(currentUser(c)).Updates(map[string]interface{}{
		"pgp_public_key":  "",
		"pgp_private_key": "",
		"pgp_fingerprint": "",
	}).Error; err != nil {
		return core.Fail(c, 400, err, "delete failed")
	}
	return c.SendStatus(204)
}

// pgpLookup returns the public key of another local user, so the composer can
// encrypt to a colleague automatically. When the address has no key in the
// users table, the caller's imported keyring is consulted next.
// pgpLookup returns a public key for an email address.
// @Summary Lookup public key
// @Tags pgp
// @Produce json
// @Param email query string true "address"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} models.APIError
// @Router /pgp/key [get]
func (h *Handler) pgpLookup(c *fiber.Ctx) error {
	email := strings.ToLower(strings.TrimSpace(c.Query("email")))
	if email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email is required"})
	}
	var u struct {
		PGPPublicKey   string
		PGPFingerprint string
	}
	if err := h.DB.Model(&models.User{}).Select("pgp_public_key, pgp_fingerprint").Where("lower(email) = ?", email).Scan(&u).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	if u.PGPPublicKey != "" {
		return c.JSON(fiber.Map{"public_key": u.PGPPublicKey, "fingerprint": u.PGPFingerprint, "source": "user"})
	}
	// Fall back to the caller's imported keyring for external addresses.
	var key models.PGPKey
	if err := h.DB.Where("user_email = ? AND email = ?", currentUser(c).Email, email).Order("id desc").First(&key).Error; err == nil {
		return c.JSON(fiber.Map{"public_key": key.PublicKey, "fingerprint": key.Fingerprint, "source": "keyring"})
	}
	return c.Status(404).JSON(fiber.Map{"error": "no public key for this address"})
}

// pgpKeys lists the current user's imported keyring entries.
// @Summary List keyring
// @Tags pgp
// @Produce json
// @Success 200 {array} models.PGPKey
// @Router /me/pgp/keys [get]
func (h *Handler) pgpKeys(c *fiber.Ctx) error {
	var keys []models.PGPKey
	if err := h.DB.Where("user_email = ?", currentUser(c).Email).Order("created_at desc").Find(&keys).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(keys)
}

// pgpImportKey stores an armored public key in the current user's keyring.
// @Summary Import public key
// @Tags pgp
// @Accept json
// @Produce json
// @Success 201 {object} models.PGPKey
// @Failure 400 {object} models.APIError
// @Router /me/pgp/keys [post]
func (h *Handler) pgpImportKey(c *fiber.Ctx) error {
	var in struct {
		Email     string `json:"email"`
		PublicKey string `json:"public_key"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.PublicKey) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "public_key is required"})
	}
	info, err := pgp.ParsePublicKey(in.PublicKey)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		if len(info.Emails) > 0 {
			email = info.Emails[0]
		} else {
			return c.Status(400).JSON(fiber.Map{"error": "email is required"})
		}
	}
	var count int64
	if err := h.DB.Model(&models.PGPKey{}).
		Where("user_email = ? AND email = ? AND fingerprint = ?", currentUser(c).Email, email, info.Fingerprint).
		Count(&count).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	if count > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "key already imported"})
	}
	key := models.PGPKey{
		UserEmail:   currentUser(c).Email,
		Email:       email,
		PublicKey:   in.PublicKey,
		Fingerprint: info.Fingerprint,
	}
	if err := h.DB.Create(&key).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.Status(201).JSON(key)
}

// pgpDeleteKey removes an imported key from the keyring.
// @Summary Remove keyring entry
// @Tags pgp
// @Success 204
// @Router /me/pgp/keys/{id} [delete]
func (h *Handler) pgpDeleteKey(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Where("id = ? AND user_email = ?", uint(id), currentUser(c).Email).Delete(&models.PGPKey{})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "internal error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.SendStatus(204)
}

// pgpEncrypt seals text for an armored public key.
// pgpEncrypt encrypts a message for a recipient's public key.
// @Summary Encrypt message
// @Tags pgp
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/pgp/encrypt [post]
func (h *Handler) pgpEncrypt(c *fiber.Ctx) error {
	var in struct {
		Text      string `json:"text"`
		PublicKey string `json:"public_key"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" || in.PublicKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text and public_key are required"})
	}
	encrypted, err := pgp.Encrypt(in.PublicKey, in.Text)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"encrypted": encrypted})
}

// pgpDecrypt opens an armored message with the current user's private key.
// pgpDecrypt decrypts a message with the user's private key.
// @Summary Decrypt message
// @Tags pgp
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/pgp/decrypt [post]
func (h *Handler) pgpDecrypt(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.PGPPrivateKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no private key configured"})
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	priv, err := crypto.Decrypt(h.Cfg.SecretKey, u.PGPPrivateKey)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "private key unlock failed"})
	}
	plaintext, err := pgp.Decrypt(priv, in.Text)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"plaintext": plaintext})
}

// pgpSign signs text with the current user's private key.
// pgpSign signs a message with the user's private key.
// @Summary Sign message
// @Tags pgp
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/pgp/sign [post]
func (h *Handler) pgpSign(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.PGPPrivateKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no private key configured"})
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	priv, err := crypto.Decrypt(h.Cfg.SecretKey, u.PGPPrivateKey)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "private key unlock failed"})
	}
	signature, err := pgp.Sign(priv, in.Text)
	if err != nil {
		return core.Fail(c, 400, err, "sign failed")
	}
	return c.JSON(fiber.Map{"signature": signature})
}

// pgpVerify checks a detached signature against a public key.
// pgpVerify verifies a signature against a public key.
// @Summary Verify signature
// @Tags pgp
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/pgp/verify [post]
func (h *Handler) pgpVerify(c *fiber.Ctx) error {
	var in struct {
		Text      string `json:"text"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" || in.Signature == "" || in.PublicKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text, signature and public_key are required"})
	}
	valid, err := pgp.Verify(in.PublicKey, in.Text, in.Signature)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"valid": valid})
}
