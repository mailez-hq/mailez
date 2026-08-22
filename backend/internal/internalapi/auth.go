package internalapi

import (
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/models"
	"mailez/backend/internal/password"
)

// webmailPorts are the internal ports reserved for webmail traffic; temp tokens
// are only accepted there (mirrors the reference implementation's WEBMAIL_PORTS).
var webmailPorts = map[string]bool{"14190": true, "10143": true, "10025": true}

// statuses maps error kinds to per-protocol error messages/codes.
var statuses = map[string]map[string]string{
	"authentication": {
		"imap": "AUTHENTICATIONFAILED", "smtp": "535 5.7.8", "submission": "535 5.7.8",
		"lmtp": "535 5.7.8", "pop3": "-ERR Authentication failed", "sieve": "AuthFailed",
	},
	"ratelimit": {
		"imap": "LIMIT", "smtp": "451 4.3.2", "submission": "451 4.3.2",
		"lmtp": "451 4.3.2", "pop3": "-ERR [LOGIN-DELAY] Retry later", "sieve": "AuthFailed",
	},
}

// authUser is the SSO gate for webmail and other authenticated surfaces.
// Returns X-User / X-User-Token headers when a valid session cookie is present.
func (h *Handler) authUser(c *fiber.Ctx) error {
	sid := c.Cookies(h.Auth.SessionName)
	user, err := h.Auth.UserFromSession(c.Context(), sid)
	if err != nil || user == nil || !user.Enabled {
		return c.SendStatus(fiber.StatusForbidden)
	}
	token, err := h.Auth.CreateTempToken(c.Context(), user.Email, sid)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Set("X-User", user.Email)
	c.Set("X-User-Token", token)
	return c.SendStatus(fiber.StatusOK)
}

// authAdmin fails unless the session belongs to an enabled global admin.
func (h *Handler) authAdmin(c *fiber.Ctx) error {
	sid := c.Cookies(h.Auth.SessionName)
	user, err := h.Auth.UserFromSession(c.Context(), sid)
	if err != nil || user == nil || !user.Enabled || !user.GlobalAdmin {
		return c.SendStatus(fiber.StatusForbidden)
	}
	return c.SendStatus(fiber.StatusOK)
}

// authBasic validates an HTTP Basic Authorization header (used by webdav).
func (h *Handler) authBasic(c *fiber.Ctx) error {
	authz := c.Get("Authorization")
	if !strings.HasPrefix(authz, "Basic ") {
		c.Set("WWW-Authenticate", `Basic realm="Login Required"`)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authz, "Basic "))
	if err != nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	email, pw, ok := strings.Cut(string(raw), ":")
	if !ok {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	var user models.User
	if err := h.DB.First(&user, "email = ?", email).Error; err != nil || !user.Enabled || !password.Verify(user.Password, pw) {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Set("X-User", user.Email)
	return c.SendStatus(fiber.StatusOK)
}

// authEmail is the nginx mail proxy authentication endpoint.
func (h *Handler) authEmail(c *fiber.Ctx) error {
	method := strings.ToLower(c.Get("Auth-Method"))
	protocol := strings.ToLower(c.Get("Auth-Protocol"))
	authPort := c.Get("Auth-Port")

	// Incoming mail: no authentication, just route to the right backend.
	if (method == "" || method == "none") && (protocol == "smtp" || protocol == "lmtp") {
		server, port := h.serverFor(protocol, false)
		c.Set("Auth-Status", "OK")
		c.Set("Auth-Server", server)
		c.Set("Auth-Port", port)
		return c.SendStatus(fiber.StatusOK)
	}

	if method != "plain" && method != "login" {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	userEmail, _ := url.QueryUnescape(c.Get("Auth-User"))
	authPass, _ := url.QueryUnescape(c.Get("Auth-Pass"))
	clientIP, _ := url.QueryUnescape(c.Get("Client-Ip"))

	var user models.User
	userFound := h.DB.First(&user, "email = ?", userEmail).Error == nil

	if userFound && h.checkCredentials(&user, authPass, clientIP, protocol, authPort, c) {
		server, port := h.serverFor(protocol, true)
		c.Set("Auth-Status", "OK")
		c.Set("Auth-Server", server)
		c.Set("Auth-User", userEmail)
		c.Set("Auth-User-Exists", "True")
		c.Set("Auth-Port", port)
		return c.SendStatus(fiber.StatusOK)
	}

	c.Set("Auth-Status", "Authentication credentials invalid")
	c.Set("Auth-Error-Code", statuses["authentication"][protocol])
	c.Set("Auth-User", userEmail)
	c.Set("Auth-User-Exists", map[bool]string{true: "True", false: "False"}[userFound])
	c.Set("Auth-Wait", "0")
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) checkCredentials(u *models.User, pw, ip, protocol, authPort string, c *fiber.Ctx) bool {
	if !u.Enabled {
		return false
	}
	if protocol == "imap" && !u.EnableImap && !webmailPorts[authPort] {
		return false
	}
	if protocol == "pop3" && !u.EnablePop {
		return false
	}
	if webmailPorts[authPort] && strings.HasPrefix(pw, "token-") {
		return h.Auth.VerifyTempToken(c.Context(), u.Email, pw)
	}
	if auth.IsAppToken(pw) {
		var tokens []models.Token
		if err := h.DB.Where("user_email = ?", u.Email).Find(&tokens).Error; err != nil {
			return false
		}
		for _, t := range tokens {
			if password.VerifyPBKDF2SHA256(t.Password, pw) {
				return true
			}
		}
		return false
	}
	return password.Verify(u.Password, pw)
}

// serverFor resolves the backend host:port for a protocol (mirrors the reference implementation's
// get_server). Hosts come from env, defaulting to the compose service names.
func (h *Handler) serverFor(protocol string, authenticated bool) (string, string) {
	imapAddr := h.Cfg.ImapAddress
	smtpAddr := h.Cfg.SmtpAddress
	switch protocol {
	case "imap":
		return imapAddr, "143"
	case "pop3":
		return imapAddr, "110"
	case "smtp":
		if authenticated {
			return smtpAddr, "10025"
		}
		return smtpAddr, "25"
	case "submission":
		return smtpAddr, "10025"
	case "lmtp":
		return imapAddr, "2525"
	case "sieve":
		return imapAddr, "4190"
	}
	return imapAddr, "143"
}
