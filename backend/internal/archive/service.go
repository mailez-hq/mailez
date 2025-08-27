package archive

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Service owns the archive store and its admin/ingest API.
type Service struct {
	*core.App
}

// New builds the archive service.
func New(app *core.App) *Service {
	return &Service{app}
}

// Register mounts the admin archive routes (global admin only).
func (s *Service) Register(r fiber.Router) {
	r.Get("/archive/settings", s.RequireGlobalAdmin, s.listSettings)
	r.Put("/archive/settings", s.RequireGlobalAdmin, s.updateSettings)
	r.Get("/archive/messages", s.RequireGlobalAdmin, s.listMessages)
	r.Get("/archive/messages/:id", s.RequireGlobalAdmin, s.getMessage)
	r.Get("/archive/messages/:id/raw", s.RequireGlobalAdmin, s.rawMessage)
	r.Post("/archive/messages/:id/review", s.RequireGlobalAdmin, s.reviewMessage)
	r.Delete("/archive/messages/:id", s.RequireGlobalAdmin, s.deleteMessage)
	r.Get("/archive/export", s.RequireGlobalAdmin, s.exportMessages)
}

// RegisterStack mounts the internal ingest endpoint used by the mail engine.
func (s *Service) RegisterStack(r fiber.Router) {
	r.Post("/archive", s.ingest)
}

// RunRetention deletes messages whose retention deadline has passed, and
// cleans up stale per-domain overrides for domains that no longer exist.
func (s *Service) RunRetention(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	run := func() {
		res := s.DB.WithContext(ctx).
			Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
			Delete(&models.ArchivedMessage{})
		if res.Error != nil {
			log.Printf("archive retention: %v", res.Error)
			return
		}
		if res.RowsAffected > 0 {
			log.Printf("archive retention: purged %d expired messages", res.RowsAffected)
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// effectivePolicy returns the policy for a domain, applying the per-domain
// override before the global default.
func (s *Service) effectivePolicy(db *gorm.DB, domain string) (*models.ArchiveSettings, error) {
	var d models.ArchiveSettings
	err := db.Where("domain = ?", domain).First(&d).Error
	if err == nil {
		return &d, nil
	}
	if !isNotFound(err) {
		return nil, err
	}
	var g models.ArchiveSettings
	if err := db.Where("domain = ?", "").First(&g).Error; err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &g, nil
}
