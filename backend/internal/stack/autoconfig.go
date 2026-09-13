// Client autoconfiguration endpoints consumed by the nginx gateway:
// Thunderbird autoconfig XML, Outlook autodiscover (XML + JSON) and the
// Apple mobileconfig profile. The gateway rewrites the public URLs to
// /stack/autoconfig/*; the handlers answer with the deployment's public
// hostname and ports so clients configure themselves from an email address
// alone. Also serves the MTA-STS policy file.
package stack

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

func (h *Handler) registerAutoconfig(r fiber.Router) {
	r.Get("/autoconfig/mozilla", h.autoconfigMozilla)
	r.All("/autoconfig/microsoft", h.autoconfigMicrosoftXML)
	r.All("/autoconfig/microsoft.json", h.autoconfigMicrosoftJSON)
	r.Get("/autoconfig/apple", h.autoconfigApple)
}

// AutoconfigMozilla is the root-route entry for the Thunderbird XML (mounted
// at /.well-known/autoconfig/mail/config-v1.1.xml and /mail/config-v1.1.xml).
func (h *Handler) AutoconfigMozilla(c *fiber.Ctx) error { return h.autoconfigMozilla(c) }

// mailDomain derives the mail domain from the Host header, accepting the
// autoconfig./autodiscover./mta-sts. service prefixes, and confirms it is a
// served domain (or a configured alternative). Unknown hosts fall back to
// the deployment's primary domain.
func (h *Handler) mailDomain(c *fiber.Ctx) string {
	host := strings.ToLower(c.Hostname())
	host = strings.TrimSuffix(host, ".")
	// Strip port and any service prefix.
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	for _, prefix := range []string{"autoconfig.", "autodiscover.", "mta-sts.", "www."} {
		if strings.HasPrefix(host, prefix) {
			host = strings.TrimPrefix(host, prefix)
			break
		}
	}
	if host == "" {
		return h.Cfg.Domain
	}
	var count int64
	h.DB.Model(&models.Domain{}).Where("name = ?", host).Count(&count)
	if count == 0 {
		var alt models.Alternative
		if err := h.DB.First(&alt, "name = ?", host).Error; err != nil {
			return h.Cfg.Domain
		}
		return alt.DomainName
	}
	return host
}

// autoconfigMozilla serves the Thunderbird autoconfig XML.
func (h *Handler) autoconfigMozilla(c *fiber.Ctx) error {
	domain := h.mailDomain(c)
	c.Set(fiber.HeaderContentType, "application/xml")
	return c.SendString(`<?xml version="1.0"?>
<clientConfig version="1.1">
  <emailProvider id="` + domain + `">
    <domain>` + domain + `</domain>
    <displayName>Mailez</displayName>
    <displayShortName>Mailez</displayShortName>
    <incomingServer type="imap">
      <hostname>` + h.Cfg.Hostname + `</hostname>
      <port>` + itoa(h.Cfg.PublicImapPort) + `</port>
      <socketType>SSL</socketType>
      <authentication>password-cleartext</authentication>
      <username>%EMAILADDRESS%</username>
    </incomingServer>
    <outgoingServer type="smtp">
      <hostname>` + h.Cfg.Hostname + `</hostname>
      <port>` + itoa(h.Cfg.PublicSmtpPort) + `</port>
      <socketType>STARTTLS</socketType>
      <authentication>password-cleartext</authentication>
      <username>%EMAILADDRESS%</username>
    </outgoingServer>
  </emailProvider>
</clientConfig>`)
}

// autoconfigMicrosoftXML serves the classic Outlook autodiscover response
// with IMAP/SMTP settings (the EAS variant lives in the ActiveSync module).
func (h *Handler) autoconfigMicrosoftXML(c *fiber.Ctx) error {
	domain := h.mailDomain(c)
	c.Set(fiber.HeaderContentType, "text/xml")
	return c.SendString(`<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">
  <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
    <User>
      <DisplayName>` + domain + `</DisplayName>
    </User>
    <Account>
      <AccountType>email</AccountType>
      <Action>settings</Action>
      <Protocol>
        <Type>IMAP</Type>
        <Server>` + h.Cfg.Hostname + `</Server>
        <Port>` + itoa(h.Cfg.PublicImapPort) + `</Port>
        <DomainRequired>off</DomainRequired>
        <LoginName>%EMAILADDRESS%</LoginName>
        <SSL>on</SSL>
        <AuthRequired>on</AuthRequired>
      </Protocol>
      <Protocol>
        <Type>SMTP</Type>
        <Server>` + h.Cfg.Hostname + `</Server>
        <Port>` + itoa(h.Cfg.PublicSmtpPort) + `</Port>
        <DomainRequired>off</DomainRequired>
        <LoginName>%EMAILADDRESS%</LoginName>
        <SSL>on</SSL>
        <AuthRequired>on</AuthRequired>
      </Protocol>
    </Account>
  </Response>
</Autodiscover>`)
}

// autoconfigMicrosoftJSON serves the newer Outlook JSON variant.
func (h *Handler) autoconfigMicrosoftJSON(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/json")
	return c.SendString(`{"accounts":[{"protocols":[` +
		`{"protocol":"imap","host":"` + h.Cfg.Hostname + `","port":` + strconv.Itoa(h.Cfg.PublicImapPort) + `,"ssl":true},` +
		`{"protocol":"smtp","host":"` + h.Cfg.Hostname + `","port":` + strconv.Itoa(h.Cfg.PublicSmtpPort) + `,"ssl":true,"starttls":true}` +
		`]}]}`)
}

// autoconfigApple serves an installable account profile for Apple clients.
func (h *Handler) autoconfigApple(c *fiber.Ctx) error {
	domain := h.mailDomain(c)
	email := c.Query("email")
	c.Set(fiber.HeaderContentType, "application/x-apple-aspen-config")
	return c.SendString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>MailAccountType</key><string>EmailTypeIMAP</string>
      <key>EmailAccountName</key><string>` + email + `</string>
      <key>EmailAddress</key><string>` + email + `</string>
      <key>IncomingMailServerHostName</key><string>` + h.Cfg.Hostname + `</string>
      <key>IncomingMailServerPort</key><integer>` + itoa(h.Cfg.PublicImapPort) + `</integer>
      <key>IncomingMailServerUseSSL</key><true/>
      <key>OutgoingMailServerHostName</key><string>` + h.Cfg.Hostname + `</string>
      <key>OutgoingMailServerPort</key><integer>` + itoa(h.Cfg.PublicSmtpPort) + `</integer>
      <key>OutgoingMailServerUseSSL</key><true/>
      <key>OutgoingPasswordSameAsIncoming</key><true/>
      <key>PayloadType</key><string>com.apple.mail.managed</string>
    </dict>
  </array>
  <key>PayloadDisplayName</key><string>Mailez ` + domain + `</string>
</dict>
</plist>`)
}

// MtaStsPolicy serves /.well-known/mta-sts.txt for the receiving domain
// derived from the Host header. Mode "testing" is deliberately safe: sending
// servers only report failures without enforcing TLS.
func (h *Handler) MtaStsPolicy(c *fiber.Ctx) error {
	// Validates the Host-derived domain is served here (and falls back to
	// the primary domain otherwise); the policy itself advertises the
	// deployment's MX hostname.
	_ = h.mailDomain(c)
	c.Set(fiber.HeaderContentType, "text/plain")
	return c.SendString("version: STSv1\n" +
		"mode: testing\n" +
		"mx: " + h.Cfg.Hostname + "\n" +
		"max_age: 86400\n")
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
