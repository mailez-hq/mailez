//go:build mailez_ee

// Enterprise-edition wiring: mounts the internal/ee route surfaces and
// starts the compliance workers. Everything here compiles only with the
// mailez_ee tag; the community export strips the file and the packages.
package server

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/compose"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/ee/activesync"
	"mailez/backend/internal/ee/ai"
	"mailez/backend/internal/ee/announcement"
	"mailez/backend/internal/ee/archive"
	"mailez/backend/internal/ee/delegation"
	"mailez/backend/internal/ee/dlp"
	eeldap "mailez/backend/internal/ee/ldap"
)

// eeNewDirectorySync builds the AD/LDAP integration (login fallback,
// auto-provisioning, address-book sync).
func eeNewDirectorySync(db *gorm.DB, secretKey string) core.DirectorySync {
	return eeldap.New(db, secretKey)
}

// eeWireAppDirectory wires the directory integration into the app shell and
// enforces the licensed mailbox cap on auto-provisioning.
func eeWireAppDirectory(s *Server, app *core.App) {
	app.LDAP = s.LDAP
	if s.LDAP != nil {
		s.LDAP.SetCapacityChecker(app.License.CheckCapacity)
	}
}

// eePublicBranding fills the public settings brand from the enterprise
// white-label configuration (admin console → config → branding).
func eePublicBranding(s *Server, brand *publicBranding) {
	var row models.BrandingConfig
	if err := s.DB.First(&row).Error; err != nil {
		return
	}
	brand.Title = row.Title
	brand.Subtitle = row.Subtitle
	brand.Tagline = row.Tagline
	brand.Feature1 = row.Feature1
	brand.Feature2 = row.Feature2
	brand.Feature3 = row.Feature3
	brand.LogoURL = row.LogoURL
	brand.HeroURL = row.HeroURL
	brand.Copyright = row.Copyright
	brand.Contact = row.Contact
}

// eeRegisterRoutes mounts the enterprise API surfaces: delegation,
// announcements, compliance archive, DLP, AI assistant and Exchange
// ActiveSync.
func eeRegisterRoutes(s *Server, app *core.App, v1, authed, stackGroup fiber.Router) {
	delegation.New(app).Register(authed)
	announcement.New(app).Register(authed)
	archive.New(app).Register(authed)
	dlp.New(app).Register(authed)
	ai.RegisterAPI(authed, app, ai.New(s.DB, s.Cfg))
	// ActiveSync (mobile device sync): WBXML commands at
	// /Microsoft-Server-ActiveSync plus autodiscover.
	activesync.New(s.DB, s.Auth, s.Cfg, app.Mail, s.basicAuthCache).Register(s.App)

	// Internal /stack endpoints consumed by the engine (archive ingest,
	// DLP verdict push).
	archive.New(app).RegisterStack(stackGroup)
	dlp.New(app).RegisterStack(stackGroup)
}

// outboundScannerAdapter converts the DLP service result into the compose
// seam's verdict type.
type outboundScannerAdapter struct{ svc *dlp.Service }

func (a outboundScannerAdapter) CheckRaw(ctx context.Context, senderEmail, from string, to []string, raw []byte) (*compose.OutboundVerdict, error) {
	res, err := a.svc.CheckRaw(ctx, senderEmail, from, to, raw)
	if err != nil || res == nil {
		return nil, err
	}
	return &compose.OutboundVerdict{Action: res.Action, Reason: res.Reason, ID: res.ID}, nil
}

// eeStartComplianceWorkers builds the DLP service, starts its expiry sweep
// and the archive retention sweep, and returns the outbound scanner seam.
func eeStartComplianceWorkers(db *gorm.DB, am *auth.Manager, cfg core.Config, bgCtx context.Context) compose.OutboundScanner {
	appShell := &core.App{DB: db, Auth: am, Cfg: cfg}
	dlpSvc := dlp.New(appShell)
	go dlpSvc.RunExpiry(bgCtx)
	// Compliance archive retention: purge messages past their policy
	// deadline.
	go archive.New(appShell).RunRetention(bgCtx)
	// Organization address book refresh (AD/LDAP) when directory sync is on.
	go eeldap.RunSyncWorker(bgCtx, db, cfg.SecretKey)
	return outboundScannerAdapter{svc: dlpSvc}
}
