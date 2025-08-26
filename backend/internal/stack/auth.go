package stack

import (
	"encoding/base64"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

// webmailPorts are the internal ports reserved for webmail traffic; temp
// tokens are only accepted on these ports. The postdove engine reaches the
// gateway proxies on 1143/11490/1587; mailezine publishes the engine ports
// itself (143 imap, 4190 managesieve, 1587 submission), so those must be
// accepted for the temp-token path too.
var webmailPorts = map[string]bool{
	"11490": true, // gateway managesieve proxy
	"1143":  true, // gateway imap proxy
	"1587":  true, // submission (both engines)
	"143":   true, // mailezine imap direct
	"4190":  true, // mailezine managesieve direct
}

// bcryptGate caps concurrent password verifications. A cost-12 bcrypt check
// burns a full core for ~300-500ms; a login burst would otherwise saturate
// every CPU and blow tail latency into multi-second territory, making
// clients (nginx login.lua, mailezine auth client) retry into the same pile.
// Serializing through a GOMAXPROCS-sized gate keeps burst throughput
// predictable and bounded. Size is overridable via MAILEZ_AUTH_WORKERS.
var bcryptGate = make(chan struct{}, authWorkers())

func authWorkers() int {
	if v := os.Getenv("MAILEZ_AUTH_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
}

// verifyPassword runs the bcrypt verification behind the gate, honouring
// request cancellation while queued.
func verifyPassword(ctx *fiber.Ctx, hash, pw string) bool {
	select {
	case bcryptGate <- struct{}{}:
		defer func() { <-bcryptGate }()
	case <-ctx.UserContext().Done():
		return false
	}
	return password.Verify(hash, pw)
}

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
	if err := h.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error; err != nil || !user.Enabled || !password.Verify(user.Password, pw) {
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
	userFound := h.DB.WithContext(c.Context()).First(&user, "email = ?", userEmail).Error == nil

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
		if err := h.DB.WithContext(c.Context()).Where("user_email = ?", u.Email).Find(&tokens).Error; err != nil {
			return false
		}
		for _, t := range tokens {
			if password.VerifyPBKDF2SHA256(t.Password, pw) {
				return true
			}
		}
		return false
	}
	return verifyPassword(c, u.Password, pw)
}

// serverFor resolves the backend host:port for a protocol. Hosts come from
// env, defaulting to the compose
// service names; the hostname is resolved to an IP because nginx's mail auth
// module (ngx_parse_addr) only accepts IP literals in Auth-Server.
func (h *Handler) serverFor(protocol string, authenticated bool) (string, string) {
	imapAddr := h.Cfg.DovecotAddress
	smtpAddr := h.Cfg.PostfixAddress
	var host string
	switch protocol {
	case "imap":
		host = imapAddr
		return resolveHostname(host), "143"
	case "pop3":
		host = imapAddr
		return resolveHostname(host), "110"
	case "smtp":
		if authenticated {
			host = smtpAddr
			return resolveHostname(host), "1587"
		}
		host = smtpAddr
		return resolveHostname(host), "25"
	case "submission":
		host = smtpAddr
		return resolveHostname(host), "1587"
	case "lmtp":
		host = imapAddr
		return resolveHostname(host), "2525"
	case "sieve":
		host = imapAddr
		return resolveHostname(host), "4190"
	}
	return resolveHostname(imapAddr), "143"
}

// resolveHostname resolves a backend host for the mail auth module: IP
// literals pass through, hostnames are resolved with
// the system DNS (Docker's embedded DNS in containers, /etc/hosts on the
// host), preferring IPv4 like the legacy original's family-sorted getaddrinfo.
// If resolution fails the raw value is returned so callers still see a
// meaningful address instead of an empty one.
func resolveHostname(hostname string) string {
	if ip := net.ParseIP(hostname); ip != nil {
		return hostname
	}
	ips, err := net.LookupHost(hostname)
	if err != nil || len(ips) == 0 {
		return hostname
	}
	for _, ip := range ips {
		if strings.Contains(ip, ".") {
			return ip
		}
	}
	return ips[0]
}
