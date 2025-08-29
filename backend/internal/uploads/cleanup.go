package uploads

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"mailez/backend/internal/core/models"
)

// RunCleanup periodically deletes relay files past their expiry, together
// with their database rows.
func (s *Service) RunCleanup(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.cleanup()
		}
	}
}

func (s *Service) cleanup() {
	var rows []models.UploadedFile
	if err := s.DB.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).Find(&rows).Error; err != nil {
		return
	}
	if len(rows) == 0 {
		return
	}
	root, _ := s.dir()
	for _, row := range rows {
		_ = os.Remove(filepath.Join(root, row.StoredPath))
		_ = s.DB.Delete(&row).Error
	}
}
