package domain

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerDkim(r fiber.Router, mw fiber.Handler) {
	r.Get("/domains/:name/dkim", mw, h.dkimStatus)
	r.Post("/domains/:name/dkim", mw, h.dkimGenerate)
}

// dkimStatus reports whether the domain has a DKIM key and, if so, the DNS
// record that must be published for signing to verify.
// dkimStatus reports DKIM configuration for a domain.
// @Summary DKIM status
// @Tags domains
// @Produce json
// @Param name path string true "domain name"
// @Success 200 {object} map[string]interface{}
// @Router /domains/{name}/dkim [get]
func (h *Handler) dkimStatus(c *fiber.Ctx) error {
	d, err := h.findDomain(c.Params("name"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "domain not found"})
	}
	return c.JSON(h.dkimResponse(d))
}

// dkimGenerate creates a fresh RSA-2048 DKIM key pair for the domain,
// stores the private key, and returns the public DNS record. Generating again
// rotates the key (old mail in transit may fail DKIM until DNS propagates).
// dkimGenerate (re)generates the DKIM key pair for a domain.
// @Summary Generate DKIM key
// @Tags domains
// @Produce json
// @Param name path string true "domain name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /domains/{name}/dkim [post]
func (h *Handler) dkimGenerate(c *fiber.Ctx) error {
	d, err := h.findDomain(c.Params("name"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "domain not found"})
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return core.Fail(c, fiber.StatusInternalServerError, err, "key generation failed")
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err := h.DB.Model(&d).Update("dkim_key", string(privPEM)).Error; err != nil {
		return core.Fail(c, fiber.StatusInternalServerError, err, "save key failed")
	}
	_ = h.DB.First(&d, "name = ?", d.Name)
	return c.JSON(h.dkimResponse(d))
}

// dkimResponse builds the API shape for a domain's DKIM status.
func (h *Handler) dkimResponse(d models.Domain) fiber.Map {
	selector := h.Cfg.DkimSelector
	publicKey := ""
	if d.DkimKey != "" {
		publicKey = dkimPublicKeyTXT(d.DkimKey)
	}
	return fiber.Map{
		"domain":     d.Name,
		"selector":   selector,
		"enabled":    d.DkimKey != "",
		"record":     selector + "._domainkey." + d.Name,
		"public_key": publicKey,
	}
}

// dkimPublicKeyTXT derives the "v=DKIM1; k=rsa; p=..." TXT value from a stored
// PKCS#1 private key. An empty string means the stored key is unusable.
func dkimPublicKeyTXT(privPEM string) string {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return ""
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return ""
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return ""
	}
	return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pubDER)
}

func (h *Handler) findDomain(name string) (models.Domain, error) {
	var d models.Domain
	err := h.DB.First(&d, "name = ?", name).Error
	return d, err
}
