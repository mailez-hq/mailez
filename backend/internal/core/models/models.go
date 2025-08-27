package models

import (
	"time"

	"gorm.io/gorm"
)

// AutoMigrate creates or updates all mailez tables.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Config{},
		&AiConfig{},
		&Domain{},
		&Alternative{},
		&Relay{},
		&User{},
		&Alias{},
		&Token{},
		&Fetch{},
		&DomainAccess{},
		&AuditLog{},
		&Contact{},
		&PushSubscription{},
		&VapidKey{},
		&Outbox{},
		&Label{},
		&Template{},
		&CardDAVConfig{},
		&PGPKey{},
		&Webhook{},
		&SmimeCert{},
		&Account{},
		&MailDelegation{},
		&CalendarEvent{},
		&LdapConfig{},
		&OrgContact{},
		&Announcement{},
		&ArchiveSettings{},
		&ArchivedMessage{},
		&DlpRule{},
		&PendingApproval{},
		&CalendarShare{},
		&CalendarReminderLog{},
		&SchemaMigration{},
	)
}

// PushSubscription stores a Web Push endpoint for a user, plus the encrypted
// auto-generated app token the notifier uses to poll unseen counts.
type PushSubscription struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserEmail string    `gorm:"size:255;not null;index" json:"user_email"`
	Endpoint  string    `gorm:"type:text;not null" json:"endpoint"`
	P256DH    string    `gorm:"type:text;not null" json:"p256dh"`
	Auth      string    `gorm:"type:text;not null" json:"auth"`
	TokenEnc  string    `gorm:"type:text;not null" json:"-"`
	TokenID   uint      `json:"token_id"`
	CreatedAt time.Time `json:"created_at"`
}

// VapidKey holds the application-server VAPID key pair, generated on demand
// and shared by every push subscription. Both values are base64url-encoded
// raw keys (exactly what the browser expects for applicationServerKey).
type VapidKey struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	PublicKey  string `gorm:"type:text;not null" json:"-"`
	PrivateKey string `gorm:"type:text;not null" json:"-"`
}
