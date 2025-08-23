package fetch

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
)

// Handler serves the fetch (external mailbox polling) domain routes.
type Handler struct {
	*core.App
}

func currentUser(c *fiber.Ctx) *models.User { return core.CurrentUser(c) }

// RegisterAPI mounts the fetch routes.
func RegisterAPI(r fiber.Router, app *core.App) {
	h := &Handler{app}
	h.registerFetches(r, app.RequireGlobalAdmin)
}

func (h *Handler) registerFetches(r fiber.Router, mw fiber.Handler) {
	r.Get("/fetches", mw, h.listFetches)
	r.Post("/fetches", mw, h.createFetch)
	r.Put("/fetches/:id", mw, h.updateFetch)
	r.Delete("/fetches/:id", mw, h.deleteFetch)
}

// listFetches returns all fetch accounts, optionally filtered by ?user=email.
func (h *Handler) listFetches(c *fiber.Ctx) error {
	q := h.DB.Order("id")
	if user := c.Query("user"); user != "" {
		q = q.Where("user_email = ?", user)
	}
	var fetches []models.Fetch
	if err := q.Find(&fetches).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(fetches)
}

type fetchIn struct {
	UserEmail string `json:"user_email"`
	Protocol  string `json:"protocol"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	TLS       *bool  `json:"tls"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Keep      *bool  `json:"keep"`
	Scan      *bool  `json:"scan"`
	Invisible *bool  `json:"invisible"`
	Folders   string `json:"folders"`
}

func (h *Handler) createFetch(c *fiber.Ctx) error {
	var in fetchIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.UserEmail == "" || in.Host == "" || in.Username == "" || in.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "user, host, username and password are required"})
	}
	protocol := strings.ToLower(in.Protocol)
	if protocol != "imap" && protocol != "pop3" {
		return c.Status(400).JSON(fiber.Map{"error": "protocol must be imap or pop3"})
	}
	var user models.User
	if err := h.DB.First(&user, "email = ?", in.UserEmail).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	// Fetch passwords must be recoverable at runtime by the poller, so they are
	// encrypted with the secret key rather than hashed.
	enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.Password)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	f := models.Fetch{
		UserEmail: in.UserEmail,
		Protocol:  protocol,
		Host:      in.Host,
		Port:      in.Port,
		Username:  in.Username,
		Password:  enc,
		Folders:   in.Folders,
	}
	if in.TLS != nil {
		f.TLS = *in.TLS
	}
	if in.Keep != nil {
		f.Keep = *in.Keep
	}
	if in.Scan != nil {
		f.Scan = *in.Scan
	}
	if in.Invisible != nil {
		f.Invisible = *in.Invisible
	}
	if err := h.DB.Create(&f).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(f)
}

func (h *Handler) updateFetch(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var f models.Fetch
	if err := h.DB.First(&f, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "fetch not found"})
	}
	var in fetchIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Protocol != "" {
		protocol := strings.ToLower(in.Protocol)
		if protocol != "imap" && protocol != "pop3" {
			return c.Status(400).JSON(fiber.Map{"error": "protocol must be imap or pop3"})
		}
		f.Protocol = protocol
	}
	if in.Host != "" {
		f.Host = in.Host
	}
	if in.Port != 0 {
		f.Port = in.Port
	}
	if in.Username != "" {
		f.Username = in.Username
	}
	if in.Password != "" {
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.Password)
		if err != nil {
			return core.Fail(c, 500, err, "internal error")
		}
		f.Password = enc
	}
	if in.TLS != nil {
		f.TLS = *in.TLS
	}
	if in.Keep != nil {
		f.Keep = *in.Keep
	}
	if in.Scan != nil {
		f.Scan = *in.Scan
	}
	if in.Invisible != nil {
		f.Invisible = *in.Invisible
	}
	f.Folders = in.Folders
	if err := h.DB.Save(&f).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(f)
}

func (h *Handler) deleteFetch(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.DB.Delete(&models.Fetch{}, "id = ?", id).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
