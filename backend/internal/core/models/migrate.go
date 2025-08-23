package models

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// SchemaMigration records applied migrations.
type SchemaMigration struct {
	ID        string    `gorm:"primaryKey;size:255" json:"id"`
	AppliedAt time.Time `json:"applied_at"`
}

// migration is one ordered schema change. Never edit an applied migration;
// append a new entry with the next ID instead.
type migration struct {
	ID string
	Up func(db *gorm.DB) error
}

var migrations = []migration{
	{
		ID: "20260823_initial_schema",
		Up: func(db *gorm.DB) error { return AutoMigrate(db) },
	},
	{
		// Fetch deduplication cursor columns (LastUID, UIDValidity, SeenUIDLs)
		// that stop the poller from re-delivering already-fetched mail.
		ID: "20260823_fetch_dedup_cursor",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Fetch{}) },
	},
}

// Migrate applies pending migrations in order and records them in
// schema_migrations. The initial schema is established by AutoMigrate; future
// schema changes must be added here as new, immutable migrations.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&SchemaMigration{}); err != nil {
		return err
	}
	for _, m := range migrations {
		var count int64
		if err := db.Model(&SchemaMigration{}).Where("id = ?", m.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := m.Up(db); err != nil {
			return err
		}
		if err := db.Create(&SchemaMigration{ID: m.ID, AppliedAt: time.Now()}).Error; err != nil {
			return err
		}
		log.Printf("migration applied: %s", m.ID)
	}
	return nil
}
