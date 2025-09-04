// Package core holds the shared dependencies and cross-cutting HTTP helpers
// every domain package embeds. Domain handlers embed *core.App so the familiar
// h.DB / h.Auth / h.Cfg / h.Mail / h.AI accessors keep working.
package core

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/entitlements"
)

// DirectorySync is the AD/LDAP directory-integration surface: login
// fallback, auto-provisioning, and the admin sync/test endpoints. Deployments
// with directory integration supply the implementation; otherwise App.LDAP
// stays nil.
type DirectorySync interface {
	Authenticate(ctx context.Context, email, password string) (bool, error)
	EnsureLocalUser(ctx context.Context, email string) error
	TestConnection(ctx context.Context, cfg models.LdapConfig, bindPassword string) error
	SyncAccounts(ctx context.Context) (created, disabled int, err error)
	SyncContacts(ctx context.Context) (added, updated int, err error)
	SyncGroups(ctx context.Context) (created, updated, disabled int, err error)
	// ResolveGroupMembers expands an AD/LDAP distribution group for alias
	// resolution (the stack.GroupResolver seam).
	ResolveGroupMembers(ctx context.Context, groupEmail string) ([]string, error)
	// SetCapacityChecker installs the licensed-capacity guard consulted
	// before auto-provisioning a directory account.
	SetCapacityChecker(fn func(db *gorm.DB) error)
}

// App carries the dependencies shared by every domain handler.
type App struct {
	DB   *gorm.DB
	Auth *auth.Manager
	Cfg  Config
	Mail mail.Gateway
	LDAP DirectorySync
	// Service is the loaded technical-service entitlement; nil = no service
	// subscribed.
	Service *entitlements.Manager
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg Config) *App {
	svc, err := entitlements.Load(cfg.ServiceFile, cfg.Service)
	if err != nil {
		panic("service: " + err.Error())
	}
	return &App{
		DB:   db,
		Auth: authMgr,
		Cfg:  cfg,
		Mail: mail.New(cfg.MailImapAddr, cfg.MailSmtpAddr, cfg.MailSieveAddr).
			SetInsecureTLS(cfg.FetchInsecure),
		Service: svc,
	}
}

// CurrentUser returns the authenticated user set by RequireAuth.
func CurrentUser(c *fiber.Ctx) *models.User {
	u, _ := c.Locals("user").(*models.User)
	return u
}
