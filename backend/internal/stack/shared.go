package stack

import (
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// Rejections of [literal-IP] domain/address forms run on every directory
// request; compile once instead of per call.
var (
	literalIPRe      = regexp.MustCompile(`^\[.*\]$`)
	emailLiteralIPRe = regexp.MustCompile(`(^|.*@)\[.*\]$`)
)

// relayTransport builds the engine transport line for a relay domain.
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

// unsupportedAddress guards against lookups the control plane cannot resolve.
func unsupportedAddress(address string) bool {
	return strings.Count(address, "@") > 1 || strings.HasPrefix(address, `"`)
}

// sieveTemplate renders the per-user default sieve script served by the
// directory contract (/stack/directory/sieve/:email).
var sieveTemplate = template.Must(template.New("default.sieve").
	Funcs(template.FuncMap{"sieveQuote": sieveQuote, "sieveAddressTests": sieveAddressTests}).
	Parse(`require "variables";
require "vacation";
require "fileinto";
require "envelope";
require "mailbox";
require "imap4flags";
require "regex";
require "relational";
require "date";
require "comparator-i;ascii-numeric";
require "spamtestplus";
require "editheader";
require "index";

if header :index 2 :matches "Received" "from * by * for <*>; *"
{
  deleteheader "Delivered-To";
  addheader "Delivered-To" "<${3}>";
}
{{if .Whitelist}}if {{sieveAddressTests .Whitelist}}
{
  keep;
  stop;
}
{{end}}{{if .Blacklist}}if {{sieveAddressTests .Blacklist}}
{
  fileinto :create "Junk";
  stop;
}
{{end}}{{if .SpamEnabled}}
if spamtest :percent :value "gt" :comparator "i;ascii-numeric" "{{.SpamThreshold}}"
{
{{if .SpamMarkAsRead}}  setflag "\seen";
{{end}}  fileinto :create "Junk";
  stop;
}
{{end}}{{if .ReplyActive}}
if not address :localpart :contains ["From","Reply-To"] ["noreply","no-reply"]{
  vacation :days 1 {{if .DisplayedName}}:from "{{.DisplayedName | sieveQuote}} <{{.Email | sieveQuote}}>"{{end}} :subject "{{.ReplySubject | sieveQuote}}" "{{.ReplyBody | sieveQuote}}";
}
{{end}}`))

// sieveQuote escapes a value for embedding in a quoted sieve string.
func sieveQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// sieveAddressTests renders an anyof(...) test for whitelist/blacklist CSV
// entries: bare domains match via :domain, full addresses via :is.
func sieveAddressTests(csv string) string {
	parts := make([]string, 0, 4)
	for _, raw := range strings.Split(csv, ",") {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		quoted := sieveQuote(e)
		if strings.Contains(e, "@") {
			parts = append(parts, `address :is "From" "`+quoted+`"`)
		} else {
			parts = append(parts, `address :domain :is "From" "`+quoted+`"`)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "anyof (\n  " + strings.Join(parts, ",\n  ") + "\n)"
}
