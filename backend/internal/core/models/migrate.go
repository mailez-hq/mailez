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
	{
		// Outbox table backing send-undo: parked messages are delivered by a
		// background worker once their undo window elapses.
		ID: "20260824_outbox_undo_send",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Outbox{}) },
	},
	{
		// Label definitions (IMAP keyword + color) powering the tag UI.
		ID: "20260824_label_colors",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Label{}) },
	},
	{
		// Outbox Subject column backing the scheduled-send list display.
		ID: "20260824_outbox_subject",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Outbox{}) },
	},
	{
		// Webhook table backing external event callbacks (new mail, etc.).
		ID: "20260824_webhooks",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Webhook{}) },
	},
	{
		// S/MIME: own certificate columns on users + imported-cert keyring.
		ID: "20260824_smime",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&User{}, &SmimeCert{}) },
	},
	{
		// External (aggregated) IMAP/SMTP accounts.
		ID: "20260824_accounts",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Account{}) },
	},
	{
		// Contact groups/avatar columns added with the address-book P1 work;
		// existing databases need an explicit migration to gain the columns
		// (AutoMigrate on a fresh DB already creates them via the initial
		// schema).
		ID: "20260824_contact_groups_avatar",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Contact{}) },
	},
	{
		// PushSubscription gained the p256dh key column (Web Push ECDH key);
		// older databases lack it and fail every subscribe/update otherwise.
		ID: "20260824_push_subscription_p256dh",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&PushSubscription{}) },
	},
	{
		// Outbox gained account_email/account_id columns (aggregated-account
		// scheduling); older databases lack account_id and fail scheduled
		// sends.
		ID: "20260824_outbox_account_id",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Outbox{}) },
	},
	{
		// Global announcement banner (Mailu parity): one admin-authored notice
		// shown to every user in the webmail until cleared.
		ID: "20260824_announcement",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Announcement{}) },
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
