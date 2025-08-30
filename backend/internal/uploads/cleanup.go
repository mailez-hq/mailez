package uploads

import (
	"context"
	"time"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core/models"
)

// RunCleanup periodically deletes relay files past their expiry, together
// with their database rows. A lease guards it so several backend replicas
// can run without racing the same deletion.
func (s *Service) RunCleanup(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !cluster.TryHold(s.DB, "uploads_cleanup", cluster.LeaseTTL) {
				continue
			}
			s.cleanup(ctx)
		}
	}
}

func (s *Service) cleanup(ctx context.Context) {
	var rows []models.UploadedFile
	if err := s.DB.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).Find(&rows).Error; err != nil {
		return
	}
	if len(rows) == 0 {
		return
	}
	store, err := s.backend()
	if err != nil {
		return
	}
	for _, row := range rows {
		_ = store.Delete(ctx, row.StoredPath)
		_ = s.DB.Delete(&row).Error
	}
}
