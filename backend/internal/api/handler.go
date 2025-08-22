package api

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/ai"
	"mailez/backend/internal/auth"
	"mailez/backend/internal/config"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/models"
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
		Mail: mail.New(cfg.MailImapAddr, cfg.MailSmtpAddr, cfg.MailSieveAddr),
		AI:   ai.New(cfg),
	}
}

// Register mounts the v1 endpoints. signup is public; the management surface
// is split by role: self-service (any user), domain management (global admins
// and domain managers), and global admin only. Role middleware is attached per
// route (empty-prefix groups would leak it onto every route).
func (h *Handler) Register(r fiber.Router) {
	h.registerSignup(r)
	r.Use(h.requireAuth)
	r.Use(h.auditMiddleware)
	h.registerMe(r)
	h.registerContacts(r)
	h.registerAnonmail(r)
	h.registerMail(r)
	h.registerSieve(r)
	h.registerAI(r)

	h.registerDomains(r, h.requireGlobalAdmin)
	h.registerDkim(r, h.requireGlobalAdmin)
	h.registerManagers(r, h.requireGlobalAdmin)
	h.registerAlternatives(r, h.requireGlobalAdmin)
	h.registerRelays(r, h.requireGlobalAdmin)
	h.registerFetches(r, h.requireGlobalAdmin)
	h.registerTokens(r, h.requireGlobalAdmin)
	h.registerAudit(r, h.requireGlobalAdmin)
	h.registerConfig(r, h.requireGlobalAdmin)

	h.registerUsers(r, h.requireManager)
	h.registerAliases(r, h.requireManager)
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

// requireGlobalAdmin restricts a route to global administrators.
func (h *Handler) requireGlobalAdmin(c *fiber.Ctx) error {
	if !currentUser(c).GlobalAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin required"})
	}
	return c.Next()
}

// requireManager allows global admins and users holding a domain manager grant.
func (h *Handler) requireManager(c *fiber.Ctx) error {
	u := currentUser(c)
	if u.GlobalAdmin {
		return c.Next()
	}
	var count int64
	if err := h.DB.Table("manager").Where("user_email = ?", u.Email).Count(&count).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if count == 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "manager required"})
	}
	return c.Next()
}

// canManageDomain reports whether the user may manage users/aliases of a domain.
func (h *Handler) canManageDomain(u *models.User, domain string) bool {
	if u.GlobalAdmin {
		return true
	}
	var count int64
	if err := h.DB.Table("manager").Where("user_email = ? AND domain_name = ?", u.Email, domain).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// managedDomainScope returns a scoped query for the domains a manager holds.
// For global admins it returns an empty filter (no restriction).
func (h *Handler) managedDomainScope(u *models.User, q *gorm.DB) *gorm.DB {
	if u.GlobalAdmin {
		return q
	}
	var names []string
	if err := h.DB.Table("manager").Where("user_email = ?", u.Email).Pluck("domain_name", &names).Error; err != nil {
		return q.Where("1 = 0")
	}
	return q.Where("domain_name IN ?", names)
}

// currentUser returns the authenticated user set by requireAuth.
func currentUser(c *fiber.Ctx) *models.User {
	u, _ := c.Locals("user").(*models.User)
	return u
}
