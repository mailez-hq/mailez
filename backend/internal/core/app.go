// Package core holds the shared dependencies and cross-cutting HTTP helpers
// every domain package embeds. Domain handlers embed *core.App so the familiar
// h.DB / h.Auth / h.Cfg / h.Mail / h.AI accessors keep working.
package core

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/license"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/service"
)

// DirectorySync is the AD/LDAP directory-integration surface: login
// fallback, auto-provisioning, and the admin sync/test endpoints. The
// enterprise build supplies *ldap.Service (internal/ee/ldap); the community
// build leaves App.LDAP nil.
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
	// License is the loaded license manager; without a configured license it
	// is the built-in unlimited dev edition.
	License *license.Manager
	// Service is the loaded technical-service entitlement; nil = no service
	// subscribed. It is independent of the engine/edition.
	Service *service.Manager
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg Config) *App {
	var lic *license.Manager
	if strings.EqualFold(cfg.MailEngine, "mailezine") {
		// mailezine engine: the license gates mailbox capacity and refuses
		// startup when required but missing/invalid.
		m, err := license.Load(cfg.LicenseFile, cfg.License, cfg.LicenseRequired)
		if err != nil {
			panic("license: " + err.Error())
		}
		lic = m
	} else {
		// Defensive: server startup restricts MAILEZ_MAIL_ENGINE to
		// mailezine; anything reaching this branch falls back to the
		// free tier with no license enforced.
		lic = license.Community()
	}
	svc, err := service.Load(cfg.ServiceFile, cfg.Service)
	if err != nil {
		panic("service: " + err.Error())
	}
	return &App{
		DB:   db,
		Auth: authMgr,
		Cfg:  cfg,
		Mail: mail.New(cfg.MailImapAddr, cfg.MailSmtpAddr, cfg.MailSieveAddr).
			SetInsecureTLS(cfg.FetchInsecure),
		License: lic,
		Service: svc,
	}
}

// CurrentUser returns the authenticated user set by RequireAuth.
func CurrentUser(c *fiber.Ctx) *models.User {
	u, _ := c.Locals("user").(*models.User)
	return u
}
