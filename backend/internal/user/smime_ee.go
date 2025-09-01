//go:build mailez_ee

package user

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/ee/smime"
)

// registerS/MIME mounts the S/MIME surface: certificate management for the
// current user, a certificate lookup directory, and CMS encrypt/decrypt/sign/
// verify operations for the webmail composer and reading pane.
func (h *Handler) registerSmime(r fiber.Router) {
	r.Get("/me/smime", h.smimeStatus)
	r.Post("/me/smime", h.smimeImport)
	r.Delete("/me/smime", h.smimeDelete)
	r.Get("/smime/cert", h.smimeLookup)
	r.Get("/me/smime/certs", h.smimeCerts)
	r.Post("/me/smime/certs", h.smimeImportCert)
	r.Delete("/me/smime/certs/:id", h.smimeDeleteCert)
	r.Post("/mail/smime/encrypt", h.smimeEncrypt)
	r.Post("/mail/smime/decrypt", h.smimeDecrypt)
	r.Post("/mail/smime/sign", h.smimeSign)
	r.Post("/mail/smime/verify", h.smimeVerify)
}

// smimeStatus reports whether the current user has a certificate installed.
// @Summary S/MIME status
// @Tags me
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /me/smime [get]
func (h *Handler) smimeStatus(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.SmimeCert == "" {
		return c.JSON(fiber.Map{"has_cert": false})
	}
	_, info, err := smime.ParseCertificate(u.SmimeCert)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(fiber.Map{
		"has_cert":    true,
		"email":       u.SmimeEmail,
		"fingerprint": u.SmimeFingerprint,
		"subject":     info.Subject,
		"issuer":      info.Issuer,
		"not_after":   u.SmimeNotAfter,
	})
}

// smimeImport installs the user's own S/MIME identity: either a PEM
// certificate plus its private key, or a base64-encoded PKCS#12 bundle.
// @Summary Import S/MIME certificate
// @Tags me
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /me/smime [post]
func (h *Handler) smimeImport(c *fiber.Ctx) error {
	u := currentUser(c)
	var in struct {
		CertPEM     string `json:"cert_pem"`
		PrivateKey  string `json:"private_key"`
		P12B64      string `json:"p12_b64"`
		P12Password string `json:"p12_password"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	var certPEM, privKeyPEM string
	if in.P12B64 != "" {
		der, err := base64.StdEncoding.DecodeString(in.P12B64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid base64 p12"})
		}
		cert, key, err := smime.ParsePKCS12(der, in.P12Password)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		certPEM = smime.CertToPEM(cert)
		pemKey, err := smime.PrivateKeyToPEM(key)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		privKeyPEM = pemKey
	} else {
		certPEM = strings.TrimSpace(in.CertPEM)
		privKeyPEM = strings.TrimSpace(in.PrivateKey)
		if certPEM == "" || privKeyPEM == "" {
			return c.Status(400).JSON(fiber.Map{"error": "cert_pem and private_key (or p12_b64) are required"})
		}
	}

	cert, info, err := smime.ParseCertificate(certPEM)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	// Ensure the private key matches the certificate before storing.
	if err := smime.KeyMatches(cert, privKeyPEM); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	encPriv, err := crypto.Encrypt(h.Cfg.SecretKey, privKeyPEM)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	email := info.Email
	if email == "" {
		email = strings.ToLower(u.Email)
	}
	if err := h.DB.Model(u).Updates(map[string]interface{}{
		"smime_cert":        certPEM,
		"smime_private_key": encPriv,
		"smime_fingerprint": info.Fingerprint,
		"smime_email":       email,
		"smime_not_after":   cert.NotAfter,
	}).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.JSON(fiber.Map{
		"email":       email,
		"fingerprint": info.Fingerprint,
		"subject":     info.Subject,
		"issuer":      info.Issuer,
		"not_after":   cert.NotAfter,
	})
}

// smimeDelete removes the current user's S/MIME identity.
// @Summary Delete S/MIME certificate
// @Tags me
// @Success 204
// @Router /me/smime [delete]
func (h *Handler) smimeDelete(c *fiber.Ctx) error {
	if err := h.DB.Model(currentUser(c)).Updates(map[string]interface{}{
		"smime_cert":        "",
		"smime_private_key": "",
		"smime_fingerprint": "",
		"smime_email":       "",
		"smime_not_after":   nil,
	}).Error; err != nil {
		return core.Fail(c, 400, err, "delete failed")
	}
	return c.SendStatus(204)
}

// smimeLookup returns a certificate for an email address: the local user's own
// certificate first, then the caller's imported keyring.
// @Summary Lookup certificate
// @Tags smime
// @Produce json
// @Param email query string true "address"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} models.APIError
// @Router /smime/cert [get]
func (h *Handler) smimeLookup(c *fiber.Ctx) error {
	email := strings.ToLower(strings.TrimSpace(c.Query("email")))
	if email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email is required"})
	}
	var u struct {
		SmimeCert        string
		SmimeFingerprint string
	}
	if err := h.DB.Model(&models.User{}).Select("smime_cert, smime_fingerprint").Where("lower(email) = ?", email).Scan(&u).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	if u.SmimeCert != "" {
		return c.JSON(fiber.Map{"cert_pem": u.SmimeCert, "fingerprint": u.SmimeFingerprint, "source": "user"})
	}
	var cert models.SmimeCert
	if err := h.DB.Where("user_email = ? AND email = ?", currentUser(c).Email, email).Order("id desc").First(&cert).Error; err == nil {
		return c.JSON(fiber.Map{"cert_pem": cert.CertPEM, "fingerprint": cert.Fingerprint, "source": "keyring"})
	}
	return c.Status(404).JSON(fiber.Map{"error": "no certificate for this address"})
}

// smimeCerts lists the current user's imported certificate keyring.
// @Summary List certificate keyring
// @Tags smime
// @Produce json
// @Success 200 {array} models.SmimeCert
// @Router /me/smime/certs [get]
func (h *Handler) smimeCerts(c *fiber.Ctx) error {
	var certs []models.SmimeCert
	if err := h.DB.Where("user_email = ?", currentUser(c).Email).Order("created_at desc").Find(&certs).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(certs)
}

// smimeImportCert stores a certificate (used to encrypt to that address).
// @Summary Import certificate
// @Tags smime
// @Accept json
// @Produce json
// @Success 201 {object} models.SmimeCert
// @Failure 400 {object} models.APIError
// @Router /me/smime/certs [post]
func (h *Handler) smimeImportCert(c *fiber.Ctx) error {
	var in struct {
		Email   string `json:"email"`
		CertPEM string `json:"cert_pem"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.CertPEM) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "cert_pem is required"})
	}
	_, info, err := smime.ParseCertificate(in.CertPEM)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		email = info.Email
	}
	if email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email is required"})
	}
	var count int64
	if err := h.DB.Model(&models.SmimeCert{}).
		Where("user_email = ? AND email = ? AND fingerprint = ?", currentUser(c).Email, email, info.Fingerprint).
		Count(&count).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	if count > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "certificate already imported"})
	}
	cert := models.SmimeCert{
		UserEmail:   currentUser(c).Email,
		Email:       email,
		CertPEM:     in.CertPEM,
		Fingerprint: info.Fingerprint,
		Subject:     info.Subject,
		Issuer:      info.Issuer,
		NotAfter:    info.NotAfter,
	}
	if err := h.DB.Create(&cert).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
	}
	return c.Status(201).JSON(cert)
}

// smimeDeleteCert removes an imported certificate from the keyring.
// @Summary Remove keyring entry
// @Tags smime
// @Success 204
// @Router /me/smime/certs/{id} [delete]
func (h *Handler) smimeDeleteCert(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Where("id = ? AND user_email = ?", uint(id), currentUser(c).Email).Delete(&models.SmimeCert{})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "internal error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.SendStatus(204)
}

// smimeEncrypt seals text for a recipient certificate.
// @Summary Encrypt message
// @Tags smime
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/smime/encrypt [post]
func (h *Handler) smimeEncrypt(c *fiber.Ctx) error {
	var in struct {
		Text    string `json:"text"`
		CertPEM string `json:"cert_pem"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" || in.CertPEM == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text and cert_pem are required"})
	}
	encrypted, err := smime.Encrypt(in.CertPEM, in.Text)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"encrypted": encrypted})
}

// smimeDecrypt opens a CMS message with the current user's private key.
// @Summary Decrypt message
// @Tags smime
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/smime/decrypt [post]
func (h *Handler) smimeDecrypt(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.SmimeCert == "" || u.SmimePrivateKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no certificate configured"})
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	priv, err := crypto.Decrypt(h.Cfg.SecretKey, u.SmimePrivateKey)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "private key unlock failed"})
	}
	plaintext, err := smime.Decrypt(priv, u.SmimeCert, in.Text)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"plaintext": plaintext})
}

// smimeSign signs text with the current user's certificate.
// @Summary Sign message
// @Tags smime
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/smime/sign [post]
func (h *Handler) smimeSign(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.SmimeCert == "" || u.SmimePrivateKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no certificate configured"})
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	priv, err := crypto.Decrypt(h.Cfg.SecretKey, u.SmimePrivateKey)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "private key unlock failed"})
	}
	signature, err := smime.Sign(u.SmimeCert, priv, in.Text)
	if err != nil {
		return core.Fail(c, 400, err, "sign failed")
	}
	return c.JSON(fiber.Map{"signature": signature})
}

// smimeVerify checks a signed message against a signer certificate.
// @Summary Verify signature
// @Tags smime
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /mail/smime/verify [post]
func (h *Handler) smimeVerify(c *fiber.Ctx) error {
	var in struct {
		Signature string `json:"signature"`
		CertPEM   string `json:"cert_pem"`
	}
	if err := c.BodyParser(&in); err != nil || in.Signature == "" || in.CertPEM == "" {
		return c.Status(400).JSON(fiber.Map{"error": "signature and cert_pem are required"})
	}
	valid, content, err := smime.Verify(in.CertPEM, in.Signature)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"valid": valid, "content": content})
}
