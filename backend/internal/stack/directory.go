package stack

// Engine-agnostic directory contract (/stack/directory/*).
//
// These endpoints expose the mailbox directory (users, domains, aliases,
// relays, senders, quota, sieve) as plain JSON concepts, independent of any
// engine protocol. Engine adapters translate between this contract and their
// own shapes: the postdove engine currently consumes the legacy
// /stack/postfix/* and /stack/dovecot/* endpoints; future engines (stalwart,
// mox) will consume this one directly.
//
// A 404 response always means "no such object"; the request was valid but the
// directory has nothing for it.

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

func (h *Handler) registerDirectory(r fiber.Router) {
	r.Get("/directory/users/:email", h.directoryUser)
	r.Get("/directory/domains/:domain", h.directoryDomain)
	r.Get("/directory/aliases/:addr", h.directoryAliases)
	r.Get("/directory/relays/:email", h.directoryRelay)
	r.Get("/directory/senders/:email", h.directorySender)
	r.Get("/directory/senders/:email/rate", h.directorySenderRate)
	r.Get("/directory/srs/:sender", h.directorySRSForward)
	r.Get("/directory/srs/restore/:recipient", h.directorySRSRestore)
	r.Get("/directory/quota/:email", h.directoryQuota)
	r.Post("/directory/quota/:email", h.directoryQuotaUpdate)
	r.Get("/directory/sieve/:email", h.directorySieve)
}

// directoryUser returns the user record in engine-neutral form.
func (h *Handler) directoryUser(c *fiber.Ctx) error {
	email, _ := url.PathUnescape(c.Params("email"))
	u := h.findUser(email)
	if u == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(fiber.Map{
		"email":              asciiEmail(u.Email),
		"enabled":            u.Enabled,
		"quotaBytes":         u.QuotaBytes,
		"quotaBytesUsed":     u.QuotaBytesUsed,
		"forwardEnabled":     u.ForwardEnabled,
		"forwardKeep":        u.ForwardKeep,
		"forwardTargets":     splitCSV(u.ForwardDestination),
		"recipientDelimiter": h.Cfg.RecipientDelimiter,
	})
}

// directoryDomain reports whether a name is served (canonical or alternative).
func (h *Handler) directoryDomain(c *fiber.Ctx) error {
	domain, _ := url.PathUnescape(c.Params("domain"))
	if regexp.MustCompile(`^\[.*\]$`).MatchString(domain) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	var d models.Domain
	if err := h.DB.WithContext(c.Context()).Where("name IN ?", domainCandidates(domain)).First(&d).Error; err != nil {
		var alt models.Alternative
		if err2 := h.DB.WithContext(c.Context()).Where("name IN ?", domainCandidates(domain)).First(&alt).Error; err2 != nil {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.JSON(fiber.Map{"isLocal": true, "name": asciiDomain(alt.DomainName)})
	}
	return c.JSON(fiber.Map{"isLocal": true, "name": asciiDomain(d.Name)})
}

// directoryAliases resolves an address to its delivery targets. A bare domain
// resolves to itself (post-office delivery).
func (h *Handler) directoryAliases(c *fiber.Ctx) error {
	addr, _ := url.PathUnescape(c.Params("addr"))
	if unsupportedAddress(addr) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	localpart, domain, ok := h.resolveDomain(addr)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if localpart == "" {
		return c.JSON(fiber.Map{"targets": []string{asciiDomain(domain)}})
	}
	if dest := h.resolveDestination(localpart, domain, false); dest != nil {
		targets := make([]string, len(dest))
		for i, d := range dest {
			targets[i] = asciiEmail(d)
		}
		return c.JSON(fiber.Map{"targets": targets})
	}
	return c.SendStatus(fiber.StatusNotFound)
}

// directoryRelay returns the relay transport for a domain.
func (h *Handler) directoryRelay(c *fiber.Ctx) error {
	email, _ := url.PathUnescape(c.Params("email"))
	if email == "*" || regexp.MustCompile(`(^|.*@)\[.*\]$`).MatchString(email) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	_, domain, _ := h.resolveDomain(email)
	var relay models.Relay
	if err := h.DB.WithContext(c.Context()).First(&relay, "name = ?", domain).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	transport, err := relayTransport(relay)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"domain": asciiDomain(domain), "transport": transport})
}

// directorySender decides whether an authenticated user may use a given
// envelope sender address. mailezine passes the authenticated user in the
// X-Auth-User header; the sender address is allowed when it belongs to the
// user (own address or an alias delivering to them), when the user holds a
// send/delegation grant for it, or when the user has allow_spoofing on the
// same domain. Without the header the lookup degrades to the sender itself,
// preserving direct (unauthenticated) contract tests.
func (h *Handler) directorySender(c *fiber.Ctx) error {
	sender, _ := url.PathUnescape(c.Params("email"))
	if unsupportedAddress(sender) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	user := strings.ToLower(strings.TrimSpace(c.Get("X-Auth-User")))
	if user == "" {
		user = strings.ToLower(sender)
	}
	localpart, domain, _ := h.resolveDomain(sender)
	allowed := false
	if localpart != "" && !allowed {
		stripped := localpart
		if h.Cfg.RecipientDelimiter != "" {
			if i := strings.IndexAny(localpart, h.Cfg.RecipientDelimiter); i >= 0 {
				stripped = localpart[:i]
			}
		}
		for _, d := range h.resolveDestination(stripped, domain, true) {
			if strings.EqualFold(d, user) {
				allowed = true
				break
			}
		}
	}
	// Mailbox delegation: the user may send as the owner they are granted.
	if !allowed {
		var dep models.MailDelegation
		if err := h.DB.WithContext(c.Context()).
			Where("LOWER(owner_email) = LOWER(?) AND LOWER(delegate_email) = LOWER(?) AND (can_send = ? OR full_access = ?)",
				sender, user, true, true).
			First(&dep).Error; err == nil {
			allowed = true
		}
	}
	// allow_spoofing lets the user themselves send as any address of their
	// own domain (the grant no longer leaks to other authenticated users).
	if !allowed {
		var u models.User
		if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", user).Error; err == nil && u.AllowSpoofing {
			if _, d, ok := h.resolveDomain(sender); ok && strings.EqualFold(d, u.DomainName) {
				allowed = true
			}
		}
	}
	if !allowed {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(fiber.Map{"allowed": true, "addresses": []string{asciiEmail(sender)}})
}

// directorySenderRate reports the outbound rate-limit state for a sender.
func (h *Handler) directorySenderRate(c *fiber.Ctx) error {
	sender, _ := url.PathUnescape(c.Params("email"))
	var u models.User
	if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", sender).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if h.rate.hit(sender) {
		return c.JSON(fiber.Map{"allowed": false, "reason": "rate limited"})
	}
	return c.JSON(fiber.Map{"allowed": true})
}

// directorySRSForward rewrites an external envelope sender into an SRS0
// address of the served domain.
func (h *Handler) directorySRSForward(c *fiber.Ctx) error {
	sender, _ := url.PathUnescape(c.Params("sender"))
	if unsupportedAddress(sender) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	localpart, domain, _ := h.resolveDomain(sender)
	var d models.Domain
	if err := h.DB.WithContext(c.Context()).First(&d, "name = ?", domain).Error; err == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(fiber.Map{"rewritten": h.srs.forward(localpart, domain, h.Cfg.Domain)})
}

// directorySRSRestore restores the original recipient of an SRS0 address.
func (h *Handler) directorySRSRestore(c *fiber.Ctx) error {
	recipient, _ := url.PathUnescape(c.Params("recipient"))
	if !isSRSAddress(recipient) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if original, ok := h.srs.reverse(recipient); ok {
		return c.JSON(fiber.Map{"original": original})
	}
	return c.SendStatus(fiber.StatusNotFound)
}

// directoryQuota returns the quota rule for a user.
func (h *Handler) directoryQuota(c *fiber.Ctx) error {
	email, _ := url.PathUnescape(c.Params("email"))
	var u models.User
	if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", email).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(fiber.Map{"limit": u.QuotaBytes, "used": u.QuotaBytesUsed})
}

// directoryQuotaUpdate persists the used-quota reported by the engine.
func (h *Handler) directoryQuotaUpdate(c *fiber.Ctx) error {
	email, _ := url.PathUnescape(c.Params("email"))
	var u models.User
	if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", email).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	var used int64
	if err := c.BodyParser(&used); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid quota"})
	}
	if err := h.DB.WithContext(c.Context()).Model(&u).Update("quota_bytes_used", used).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(nil)
}

// directorySieve renders the default sieve script for a user.
func (h *Handler) directorySieve(c *fiber.Ctx) error {
	email, _ := url.QueryUnescape(c.Params("email"))
	var u models.User
	if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", email).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	var buf strings.Builder
	if err := sieveTemplate.Execute(&buf, &u); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"name": "default", "script": buf.String()})
}
