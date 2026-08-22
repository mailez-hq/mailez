package internalapi

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) registerAutoconfig(r fiber.Router) {
	r.Get("/autoconfig/mozilla", h.autoconfigMozilla)
	r.Get("/autoconfig/microsoft.json", h.autoconfigMicrosoftJSON)
	r.Post("/autoconfig/microsoft", h.autoconfigMicrosoft)
	r.Get("/autoconfig/apple", h.autoconfigApple)
}

// autoconfigMozilla serves the Thunderbird autoconfig XML (RFC-style).
func (h *Handler) autoconfigMozilla(c *fiber.Ctx) error {
	host := h.Cfg.Hostname
	xml := fmt.Sprintf(`<?xml version="1.0"?>
<clientConfig version="1.1">
<emailProvider id="%s">
<domain>%%EMAILDOMAIN%%</domain>

<displayName>Email</displayName>
<displayShortName>Email</displayShortName>

<incomingServer type="imap">
<hostname>%s</hostname>
<port>993</port>
<socketType>SSL</socketType>
<username>%%EMAILADDRESS%%</username>
<authentication>password-cleartext</authentication>
</incomingServer>

<outgoingServer type="smtp">
<hostname>%s</hostname>
<port>465</port>
<socketType>SSL</socketType>
<username>%%EMAILADDRESS%%</username>
<authentication>password-cleartext</authentication>
<addThisServer>true</addThisServer>
<useGlobalPreferredServer>false</useGlobalPreferredServer>
</outgoingServer>

<documentation url="https://%s/admin/client">
<descr lang="en">Configure your email client</descr>
</documentation>
</emailProvider>
</clientConfig>`, host, host, host, host)
	return c.Type("xml").SendString(xml)
}

// autoconfigMicrosoftJSON answers the Autodiscover protocol probe.
func (h *Handler) autoconfigMicrosoftJSON(c *fiber.Ctx) error {
	if c.Query("Protocol", "Autodiscoverv1") != "Autodiscoverv1" {
		return c.SendStatus(fiber.StatusNotFound)
	}
	body := fmt.Sprintf(`{"Protocol":"Autodiscoverv1","Url":"https://%s/autodiscover/autodiscover.xml"}`, h.Cfg.Hostname)
	return c.Type("json").SendString(body)
}

var emailTagRe = regexp.MustCompile(`<EMailAddress>(.*?)</EMailAddress>`)

// autoconfigMicrosoft answers the Outlook Autodiscover POST.
func (h *Handler) autoconfigMicrosoft(c *fiber.Ctx) error {
	body := string(c.Body())
	if !strings.Contains(body, "Autodiscover") {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	m := emailTagRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	email := strings.TrimSpace(m[1])
	host := h.Cfg.Hostname
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">
    <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
        <Account>
            <AccountType>email</AccountType>
            <Action>settings</Action>
            <Protocol>
                <Type>IMAP</Type>
                <Server>%s</Server>
                <Port>993</Port>
                <LoginName>%s</LoginName>
                <DomainRequired>on</DomainRequired>
                <SPA>off</SPA>
                <SSL>on</SSL>
            </Protocol>
            <Protocol>
                <Type>SMTP</Type>
                <Server>%s</Server>
                <Port>465</Port>
                <LoginName>%s</LoginName>
                <DomainRequired>on</DomainRequired>
                <SPA>off</SPA>
                <SSL>on</SSL>
            </Protocol>
        </Account>
    </Response>
</Autodiscover>`, host, email, host, email)
	return c.Type("xml").SendString(xml)
}

// autoconfigApple serves an Apple mobileconfig profile.
func (h *Handler) autoconfigApple(c *fiber.Ctx) error {
	host := h.Cfg.Hostname
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
"http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
<key>PayloadContent</key>
<array>
<dict>
<key>EmailAccountDescription</key>
<string>%s</string>
<key>EmailAccountName</key>
<string>%s</string>
<key>EmailAccountType</key>
<string>EmailTypeIMAP</string>
<key>EmailAddress</key>
<string></string>
<key>IncomingMailServerAuthentication</key>
<string>EmailAuthPassword</string>
<key>IncomingMailServerHostName</key>
<string>%s</string>
<key>IncomingMailServerPortNumber</key>
<integer>993</integer>
<key>IncomingMailServerUseSSL</key>
<true/>
<key>IncomingMailServerUsername</key>
<string></string>
<key>IncomingPassword</key>
<string></string>
<key>OutgoingMailServerAuthentication</key>
<string>EmailAuthPassword</string>
<key>OutgoingMailServerHostName</key>
<string>%s</string>
<key>OutgoingMailServerPortNumber</key>
<integer>465</integer>
<key>OutgoingMailServerUseSSL</key>
<true/>
<key>OutgoingMailServerUsername</key>
<string></string>
<key>OutgoingPasswordSameAsIncomingPassword</key>
<true/>
<key>PayloadDescription</key>
<string>%s</string>
<key>PayloadDisplayName</key>
<string>%s</string>
<key>PayloadIdentifier</key>
<string>%s.email</string>
<key>PayloadOrganization</key>
<string></string>
<key>PayloadType</key>
<string>com.apple.mail.managed</string>
<key>PayloadUUID</key>
<string>72e152e2-d285-4588-9741-25bdd50c4d11</string>
<key>PayloadVersion</key>
<integer>1</integer>
</dict>
</array>
<key>PayloadDescription</key>
<string>%s - E-Mail Account Configuration</string>
<key>PayloadDisplayName</key>
<string>E-Mail Account %s</string>
<key>PayloadIdentifier</key>
<string>E-Mail Account %s</string>
<key>PayloadOrganization</key>
<string>%s</string>
<key>PayloadRemovalDisallowed</key>
<false/>
<key>PayloadType</key>
<string>Configuration</string>
<key>PayloadUUID</key>
<string>56db43a5-d29e-4609-a908-dce94d0be48e</string>
<key>PayloadVersion</key>
<integer>1</integer>
</dict>
</plist>`, h.Cfg.Domain, host, host, host, h.Cfg.Domain, host, host, host, host, host, host)
	c.Set("Content-Type", "application/x-apple-aspen-config")
	return c.SendString(xml)
}
