package ldap

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// RunSyncWorker periodically refreshes the organization address book while
// the directory integration is enabled. The interval follows the admin
// SyncMinutes setting, re-read each minute.
func RunSyncWorker(ctx context.Context, db *gorm.DB, secretKey string) {
	svc := New(db, secretKey)
	last := time.Time{}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var cfg models.LdapConfig
			if err := db.First(&cfg).Error; err != nil {
				continue
			}
			interval := time.Duration(cfg.SyncMinutes) * time.Minute
			if interval <= 0 {
				interval = time.Hour
			}
			if !cfg.Enabled || time.Since(last) < interval {
				continue
			}
			added, updated, err := svc.SyncContacts(ctx)
			if err != nil {
				log.Printf("ldap sync failed: %v", err)
			} else {
				last = time.Now()
				log.Printf("ldap org book synced: +%d updated %d", added, updated)
			}
		}
	}
}
