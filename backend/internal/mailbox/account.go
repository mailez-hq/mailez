package mailbox

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/mail"
)

// securityValues are the accepted transport-security policies for an account.
var securityValues = map[string]bool{"none": true, "starttls": true, "tls": true}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func validSecurity(s string) bool { return securityValues[strings.ToLower(s)] }

// accountInput is the shared create/update payload. Password is optional on
// update (leave empty to keep the stored one).
type accountInput struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	ImapHost     string `json:"imap_host"`
	ImapPort     int    `json:"imap_port"`
	ImapSecurity string `json:"imap_security"`
	SmtpHost     string `json:"smtp_host"`
	SmtpPort     int    `json:"smtp_port"`
	SmtpSecurity string `json:"smtp_security"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Enabled      *bool  `json:"enabled"`
}

// apply fills a model row from validated input, encrypting the password when
// provided and defaulting empty SMTP fields to the IMAP values.
func (in *accountInput) apply(a *models.Account, secret string) error {
	a.Name = strings.TrimSpace(in.Name)
	a.Email = strings.ToLower(strings.TrimSpace(in.Email))
	a.ImapHost = strings.TrimSpace(in.ImapHost)
	a.ImapPort = in.ImapPort
	a.ImapSecurity = strings.ToLower(in.ImapSecurity)
	a.Username = strings.TrimSpace(in.Username)
	if strings.TrimSpace(in.SmtpHost) == "" {
		a.SmtpHost = a.ImapHost
	} else {
		a.SmtpHost = strings.TrimSpace(in.SmtpHost)
	}
	if in.SmtpPort == 0 {
		a.SmtpPort = in.ImapPort
	} else {
		a.SmtpPort = in.SmtpPort
	}
	if strings.TrimSpace(in.SmtpSecurity) == "" {
		a.SmtpSecurity = a.ImapSecurity
	} else {
		a.SmtpSecurity = strings.ToLower(in.SmtpSecurity)
	}
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	}
	if in.Password != "" {
		enc, err := crypto.Encrypt(secret, in.Password)
		if err != nil {
			return err
		}
		a.PasswordEnc = enc
	}
	return nil
}

// validate checks the required fields and sane ranges for an account row.
func (in *accountInput) validate(a *models.Account, requirePassword bool) error {
	if a.Name == "" || a.Email == "" || a.ImapHost == "" || a.Username == "" {
		return errors.New("name, email, imap_host and username are required")
	}
	if a.Email == a.UserEmail {
		return errors.New("cannot add the primary account itself")
	}
	if !validPort(a.ImapPort) || !validPort(a.SmtpPort) {
		return errors.New("invalid port")
	}
	if !validSecurity(a.ImapSecurity) || !validSecurity(a.SmtpSecurity) {
		return errors.New("security must be none, starttls or tls")
	}
	if requirePassword && a.PasswordEnc == "" {
		return errors.New("password is required")
	}
	return nil
}

// accountTest opens an IMAP connection with the stored credentials to surface
// configuration or authentication problems without waiting for the UI.
func accountTest(d mail.Dial) error {
	_, err := mail.New("", "", "").With(d).ListFolders("", "")
	return err
}

// RegisterAccount mounts the aggregated-account management routes.
func (h *Handler) registerAccount(r fiber.Router) {
	r.Get("/accounts", h.accounts)
	r.Post("/accounts", h.accountCreate)
	r.Put("/accounts/:id", h.accountUpdate)
	r.Delete("/accounts/:id", h.accountDelete)
	r.Post("/accounts/:id/test", h.accountTestHandler)
}

// accounts lists the caller's aggregated accounts.
// @Summary List accounts
// @Tags mail
// @Produce json
// @Success 200 {array} models.Account
// @Router /accounts [get]
func (h *Handler) accounts(c *fiber.Ctx) error {
	user := currentUser(c)
	var out []models.Account
	if err := h.DB.Where("user_email = ?", user.Email).Order("created_at").Find(&out).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(out)
}

// accountCreate stores a new aggregated account with the password encrypted.
// @Summary Create account
// @Tags mail
// @Accept json
// @Success 200 {object} models.Account
// @Failure 400 {object} map[string]interface{}
// @Router /accounts [post]
func (h *Handler) accountCreate(c *fiber.Ctx) error {
	user := currentUser(c)
	var in accountInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	acc := models.Account{UserEmail: user.Email, Enabled: true, ImapPort: 993, SmtpPort: 465}
	if err := in.apply(&acc, h.Cfg.SecretKey); err != nil {
		return core.Fail(c, 500, err, "encryption error")
	}
	if err := in.validate(&acc, true); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Create(&acc).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(acc)
}

// accountUpdate edits an aggregated account (password optional).
// @Summary Update account
// @Tags mail
// @Accept json
// @Success 200 {object} models.Account
// @Failure 400 {object} map[string]interface{}
// @Router /accounts/{id} [put]
func (h *Handler) accountUpdate(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var acc models.Account
	if err := h.DB.First(&acc, "id = ? AND user_email = ?", id, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	var in accountInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := in.apply(&acc, h.Cfg.SecretKey); err != nil {
		return core.Fail(c, 500, err, "encryption error")
	}
	if err := in.validate(&acc, false); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Save(&acc).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(acc)
}

// accountDelete removes an aggregated account.
// @Summary Delete account
// @Tags mail
// @Success 204
// @Router /accounts/{id} [delete]
func (h *Handler) accountDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Where("id = ? AND user_email = ?", id, user.Email).Delete(&models.Account{})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// accountTestHandler verifies the stored credentials against the IMAP server.
// @Summary Test account connection
// @Tags mail
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /accounts/{id}/test [post]
func (h *Handler) accountTestHandler(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var acc models.Account
	if err := h.DB.First(&acc, "id = ? AND user_email = ?", id, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	pw, err := crypto.Decrypt(h.Cfg.SecretKey, acc.PasswordEnc)
	if err != nil {
		return core.Fail(c, 500, err, "decryption error")
	}
	if err := accountTest(mail.ExternalDial(acc.Email, acc.ImapHost, acc.ImapPort, acc.ImapSecurity, acc.Username, pw)); err != nil {
		_ = h.DB.Model(&acc).Update("last_error", err.Error()).Error
		return core.Fail(c, 502, err, "connection failed")
	}
	_ = h.DB.Model(&acc).Update("last_error", "").Error
	return c.JSON(fiber.Map{"ok": true})
}
