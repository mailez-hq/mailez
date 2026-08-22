package internalapi

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailess/backend/internal/models"
)

func (h *Handler) registerPostfix(r fiber.Router) {
	r.Get("/postfix/dane/:domain", h.postfixDane)
	r.Get("/postfix/domain/:domain", h.postfixDomain)
	r.Get("/postfix/mailbox/:email", h.postfixMailbox)
	r.Get("/postfix/alias/:alias", h.postfixAlias)
	r.Get("/postfix/transport/:email", h.postfixTransport)
	r.Get("/postfix/recipient/map/:recipient", h.postfixRecipientMap)
	r.Get("/postfix/sender/map/:sender", h.postfixSenderMap)
	r.Get("/postfix/sender/login/:sender", h.postfixSenderLogin)
	r.Get("/postfix/sender/rate/:sender", h.postfixSenderRate)
}

// postfixDomain maps a served domain (or alternative) to itself.
func (h *Handler) postfixDomain(c *fiber.Ctx) error {
	domain := c.Params("domain")
	if regexp.MustCompile(`^\[.*\]$`).MatchString(domain) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", domain).Error; err != nil {
		var alt models.Alternative
		if err2 := h.DB.First(&alt, "name = ?", domain).Error; err2 != nil {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.JSON(alt.DomainName)
	}
	return c.JSON(d.Name)
}

// postfixMailbox maps a user email to itself.
func (h *Handler) postfixMailbox(c *fiber.Ctx) error {
	email, _ := url.QueryUnescape(c.Params("email"))
	u := h.findUser(email)
	if u == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(u.Email)
}

// postfixAlias resolves a destination list (comma-joined) for an address.
func (h *Handler) postfixAlias(c *fiber.Ctx) error {
	alias, _ := url.QueryUnescape(c.Params("alias"))
	if unsupportedAddress(alias) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	localpart, domain, ok := h.resolveDomain(alias)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if localpart == "" {
		return c.JSON(domain)
	}
	if dest := h.resolveDestination(localpart, domain, false); dest != nil {
		return c.JSON(strings.Join(dest, ","))
	}
	return c.SendStatus(fiber.StatusNotFound)
}

// postfixTransport returns the relay transport for a domain.
func (h *Handler) postfixTransport(c *fiber.Ctx) error {
	email, _ := url.QueryUnescape(c.Params("email"))
	if email == "*" || regexp.MustCompile(`(^|.*@)\[.*\]$`).MatchString(email) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	_, domain, _ := h.resolveDomain(email)
	var relay models.Relay
	if err := h.DB.First(&relay, "name = ?", domain).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	transport, err := relayTransport(relay)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(transport)
}

// postfixSenderLogin lists senders allowed to authenticate as the given sender.
func (h *Handler) postfixSenderLogin(c *fiber.Ctx) error {
	sender, _ := url.QueryUnescape(c.Params("sender"))
	if unsupportedAddress(sender) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	localpart, domain, _ := h.resolveDomain(sender)
	destinations := map[string]bool{}
	if localpart != "" {
		stripped := localpart
		if h.Cfg.RecipientDelimiter != "" {
			if i := strings.IndexAny(localpart, h.Cfg.RecipientDelimiter); i >= 0 {
				stripped = localpart[:i]
			}
		}
		for _, d := range h.resolveDestination(stripped, domain, true) {
			destinations[d] = true
		}
	}
	// allow_spoofing lets a user send as any address within their own domain.
	var spoofers []models.User
	if err := h.DB.Where("allow_spoofing = ? AND domain_name = ?", true, domain).Find(&spoofers).Error; err == nil {
		for _, u := range spoofers {
			destinations[u.Email] = true
		}
	}
	if len(destinations) == 0 {
		return c.SendStatus(fiber.StatusNotFound)
	}
	out := make([]string, 0, len(destinations))
	for d := range destinations {
		out = append(out, d)
	}
	return c.JSON(strings.Join(out, ","))
}

// postfixSenderRate is a no-op limiter stub for now (always allowed).
func (h *Handler) postfixSenderRate(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

// SRS rewriting is not implemented yet; return 404 (no rewriting).
func (h *Handler) postfixRecipientMap(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

func (h *Handler) postfixSenderMap(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

// postfixDane answers DANE support for a domain.
func (h *Handler) postfixDane(c *fiber.Ctx) error {
	// DANE TLSA records are not queried in the scaffold; always "dane-only"
	// would break plain hosts, so respond 404 unless configured.
	return c.SendStatus(fiber.StatusNotFound)
}

// relayTransport builds the postfix transport line for a relay domain.
func relayTransport(relay models.Relay) (string, error) {
	target := strings.ToLower(relay.SMTP)
	useLMTP := false
	useMX := false
	if strings.HasPrefix(target, "mx:") {
		target = target[3:]
		useMX = true
	} else if strings.HasPrefix(target, "lmtp:") {
		target = target[5:]
		useLMTP = true
	}
	host := target
	port := ""
	if strings.HasPrefix(target, "[") {
		rest := strings.TrimPrefix(target, "[")
		if i := strings.Index(rest, "]"); i >= 0 {
			host = rest[:i]
			if suffix := rest[i+1:]; strings.HasPrefix(suffix, ":") {
				port = suffix[1:]
			}
		}
	} else if i := strings.LastIndex(target, ":"); i >= 0 {
		host = target[:i]
		port = target[i+1:]
	}
	if host == "" {
		if useLMTP {
			return "", fiber.NewError(fiber.StatusBadRequest, "lmtp needs a host")
		}
		host = relay.Name
		useMX = true
	}
	if port != "" {
		if _, err := strconv.Atoi(port); err != nil {
			return "", fiber.NewError(fiber.StatusBadRequest, "invalid port")
		}
	}
	scheme := "smtp"
	if useLMTP {
		scheme = "lmtp"
	}
	if !useLMTP && !useMX {
		host = "[" + host + "]"
	}
	out := scheme + ":" + host
	if port != "" {
		out += ":" + port
	}
	return out, nil
}

// unsupportedAddress guards against lookups Mailu cannot resolve.
func unsupportedAddress(address string) bool {
	return strings.Count(address, "@") > 1 || strings.HasPrefix(address, `"`)
}
