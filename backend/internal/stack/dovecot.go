package stack

import (
	"net/url"
	"strconv"
	"strings"
	"text/template"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

func (h *Handler) registerDovecot(r fiber.Router) {
	r.Get("/dovecot/passdb/:email", h.dovecotPassdb)
	r.Get("/dovecot/userdb/", h.dovecotUserdbList)
	r.Get("/dovecot/userdb/:email", h.dovecotUserdb)
	r.Post("/dovecot/quota/:ns/:email", h.dovecotQuota)
	r.Get("/dovecot/sieve/name/:script/:email", h.dovecotSieveName)
	r.Get("/dovecot/sieve/data/default/:email", h.dovecotSieveData)
}

// dovecotPassdb tells Dovecot to accept the auth done by the nginx proxy.
func (h *Handler) dovecotPassdb(c *fiber.Ctx) error {
	email, _ := url.PathUnescape(c.Params("email"))
	if h.findUser(email) == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(fiber.Map{
		"password":        nil,
		"nopassword":      "Y",
		"allow_real_nets": h.Cfg.Subnet,
	})
}

// dovecotUserdbList returns all enabled users (dovecot userdb iteration).
func (h *Handler) dovecotUserdbList(c *fiber.Ctx) error {
	var emails []string
	if err := h.DB.WithContext(c.Context()).Model(&models.User{}).Where("enabled = ?", true).Pluck("email", &emails).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(emails)
}

// dovecotUserdb returns the quota rule for a user.
func (h *Handler) dovecotUserdb(c *fiber.Ctx) error {
	email, _ := url.PathUnescape(c.Params("email"))
	var u models.User
	if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", email).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(fiber.Map{"quota_rule": "*:bytes=" + int64Str(u.QuotaBytes)})
}

// dovecotQuota persists the used-quota reported by Dovecot.
func (h *Handler) dovecotQuota(c *fiber.Ctx) error {
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

// dovecotSieveName returns the script name unchanged.
func (h *Handler) dovecotSieveName(c *fiber.Ctx) error {
	return c.JSON(c.Params("script"))
}

// dovecotSieveData renders the default sieve script for a user.
func (h *Handler) dovecotSieveData(c *fiber.Ctx) error {
	email, _ := url.QueryUnescape(c.Params("email"))
	var u models.User
	if err := h.DB.WithContext(c.Context()).First(&u, "email = ?", email).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	var buf strings.Builder
	if err := sieveTemplate.Execute(&buf, &u); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(buf.String())
}

var sieveTemplate = template.Must(template.New("default.sieve").
	Funcs(template.FuncMap{"sieveQuote": sieveQuote}).
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
{{if .SpamEnabled}}
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

func int64Str(n int64) string {
	return strconv.FormatInt(n, 10)
}

// sieveQuote escapes a value for embedding in a quoted sieve string.
func sieveQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
