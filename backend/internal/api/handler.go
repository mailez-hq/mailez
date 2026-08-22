package api

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailess/backend/internal/ai"
	"mailess/backend/internal/auth"
	"mailess/backend/internal/config"
	"mailess/backend/internal/mail"
	"mailess/backend/internal/models"
)

// Handler serves the management REST API v1.
type Handler struct {
	DB   *gorm.DB
	Auth *auth.Manager
	Cfg  config.Config
	Mail *mail.Client
	AI   *ai.Manager
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg config.Config) *Handler {
	return &Handler{
		DB:   db,
		Auth: authMgr,
		Cfg:  cfg,
		Mail: mail.New(cfg.MailImapAddr, cfg.MailSmtpAddr),
		AI:   ai.New(cfg),
	}
}

// Register mounts the v1 endpoints. requireAuth guards the management surface.
func (h *Handler) Register(r fiber.Router) {
	r.Use(h.requireAuth)
	h.registerDomains(r)
	h.registerUsers(r)
	h.registerAliases(r)
	h.registerMail(r)
	h.registerAI(r)
}

// requireAuth allows any authenticated (non-anonymous) session.
func (h *Handler) requireAuth(c *fiber.Ctx) error {
	sid := c.Cookies(h.Auth.SessionName)
	user, err := h.Auth.UserFromSession(c.Context(), sid)
	if err != nil || user == nil || !user.Enabled {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
	c.Locals("user", user)
	return c.Next()
}

// currentUser returns the authenticated user set by requireAuth.
func currentUser(c *fiber.Ctx) *models.User {
	u, _ := c.Locals("user").(*models.User)
	return u
}
