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
	"mailez/backend/internal/service"
	"strings"
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
	// Service is the loaded technical-service entitlement; nil = no service
	// subscribed. It is independent of the engine/edition.
	Service *service.Manager
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg Config) *App {
	var lic *license.Manager
	if strings.EqualFold(cfg.MailEngine, "mailezine") {
		// Enterprise engine: the license gates mailbox capacity and refuses
		// startup when required but missing/invalid.
		m, err := license.Load(cfg.LicenseFile, cfg.License, cfg.LicenseRequired)
		if err != nil {
			panic("license: " + err.Error())
		}
		lic = m
	} else {
		// Community edition (postdove) is free: no license is loaded or
		// enforced, so a license file on a community deployment is ignored.
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
