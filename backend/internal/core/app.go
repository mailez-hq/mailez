// Package core holds the shared dependencies and cross-cutting HTTP helpers
// every domain package embeds. Domain handlers embed *core.App so the familiar
// h.DB / h.Auth / h.Cfg / h.Mail / h.AI accessors keep working.
package core

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/ldap"
	"mailez/backend/internal/license"
	"mailez/backend/internal/mail"
)

// App carries the dependencies shared by every domain handler.
type App struct {
	DB   *gorm.DB
	Auth *auth.Manager
	Cfg  Config
	Mail mail.Gateway
	LDAP *ldap.Service
	// License is the loaded license manager; without a configured license it
	// is the built-in unlimited dev edition.
	License *license.Manager
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg Config) *App {
	lic, err := license.Load(cfg.LicenseFile, cfg.License, cfg.LicenseRequired)
	if err != nil {
		// A required-but-invalid license must refuse startup. In the default
		// (not required) path Load never errors.
		panic("license: " + err.Error())
	}
	return &App{
		DB:   db,
		Auth: authMgr,
		Cfg:  cfg,
		Mail: mail.New(cfg.MailImapAddr, cfg.MailSmtpAddr, cfg.MailSieveAddr),
		License: lic,
	}
}

// CurrentUser returns the authenticated user set by RequireAuth.
func CurrentUser(c *fiber.Ctx) *models.User {
	u, _ := c.Locals("user").(*models.User)
	return u
}
