package models

import (
	"fmt"
	"log"
	"time"

	"mailez/backend/internal/labelutil"

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
		// AI provider settings configured through the admin console.
		ID: "20260826_ai_config",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&AiConfig{}) },
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
	{
		// Hot-path query indexes: recipient resolution (users.domain_name,
		// aliases(domain_name,disabled)), admin domain listing, app-token
		// auth (tokens.user_email), fetch ownership (fetches.user_email),
		// anonmail ownership (aliases.owner_email), alternative domains, and
		// the outbox worker poll (status,send_after). AutoMigrate creates the
		// missing named indexes on existing databases.
		ID: "20260825_query_indexes",
		Up: func(db *gorm.DB) error {
			return db.AutoMigrate(&User{}, &Alias{}, &Alternative{}, &Token{}, &Fetch{}, &Outbox{})
		},
	},
	{
		// Label keyword column: IMAP keywords must be ASCII atoms, so labels
		// with non-ASCII display names (Chinese etc.) are =XX-encoded on the
		// wire. Backfill the keyword for every existing row.
		ID: "20260826_label_keyword",
		Up: func(db *gorm.DB) error {
			if err := db.AutoMigrate(&Label{}); err != nil {
				return err
			}
			var labels []Label
			if err := db.Where("keyword = ?", "").Find(&labels).Error; err != nil {
				return err
			}
			for i := range labels {
				kw := labelutil.EncodeKeyword(labels[i].Name)
				if err := db.Model(&Label{}).Where("id = ?", labels[i].ID).
					Update("keyword", kw).Error; err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// Reusable compose templates (canned responses / 常用语) per account.
		ID: "20260827_email_templates",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&Template{}) },
	},
	{
		// Remote CardDAV address book config for one-way import sync.
		ID: "20260827_carddav",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&CardDAVConfig{}) },
	},
	{
		// Mailbox delegation / shared mailbox: a delegate may send as the
		// owner (CanSend) and optionally access the owner's full mailbox
		// (FullAccess). Powering the webmail account switcher and the
		// send-as identity list.
		ID: "20260827_mail_delegations",
		Up: func(db *gorm.DB) error { return db.AutoMigrate(&MailDelegation{}) },
	},
	{
		// Built-in CalDAV server: calendar events stored raw ICS + structured
		// columns. Contacts gain CardDAV sync state (DavUID/DavETag/DavRev).
		ID: "20260827_caldav_calendar",
		Up: func(db *gorm.DB) error {
			if err := db.AutoMigrate(&CalendarEvent{}); err != nil {
				return err
			}
			if err := db.AutoMigrate(&Contact{}); err != nil {
				return err
			}
			// Backfill a stable UID for contacts created before CardDAV
			// existed (the (user_email, dav_uid) unique index rejects empty
			// duplicates).
			var legacy []Contact
			if err := db.Where("dav_uid = ?", "").Find(&legacy).Error; err != nil {
				return err
			}
			for i := range legacy {
				uid := "mailez-" + fmt.Sprintf("%d", legacy[i].ID) + "-" + legacy[i].Email
				if err := db.Model(&Contact{}).Where("id = ?", legacy[i].ID).Update("dav_uid", uid).Error; err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// AD/LDAP directory integration: connection config (encrypted bind
		// password) and the synced read-only organization address book.
		ID: "20260827_ldap",
		Up: func(db *gorm.DB) error {
			return db.AutoMigrate(&LdapConfig{}, &OrgContact{})
		},
	},
	{
		// LDAP lifecycle: User.LdapManaged marks directory-provisioned
		// accounts so the sync worker can disable leavers safely.
		ID: "20260827_ldap_user_managed",
		Up: func(db *gorm.DB) error {
			return db.AutoMigrate(&User{})
		},
	},
	{
		// LDAP groups -> distribution-list aliases: LdapConfig gains AD
		// mapping (UPN/email-domain) and group-sync settings; Alias gains the
		// LdapGroup ownership flag.
		ID: "20260827_ldap_groups_ad",
		Up: func(db *gorm.DB) error {
			return db.AutoMigrate(&LdapConfig{}, &Alias{})
		},
	},
	{
		// Compliance email archiving: capture/retention policies and the
		// archived message store (Coremail-style 归档/检索/审查).
		ID: "20260827_email_archive",
		Up: func(db *gorm.DB) error {
			if err := db.AutoMigrate(&ArchiveSettings{}, &ArchivedMessage{}); err != nil {
				return err
			}
			// Seed an enabled global policy so a fresh install captures mail
			// out of the box; admins tune it (or disable it) in the console.
			var count int64
			if err := db.Model(&ArchiveSettings{}).Where("domain = ?", "").Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return db.Create(&ArchiveSettings{
					Domain:          "",
					Enabled:         true,
					CaptureInbound:  true,
					CaptureOutbound: true,
					RetentionDays:   0,
				}).Error
			}
			return nil
		},
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
