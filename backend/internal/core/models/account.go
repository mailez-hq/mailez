package models

import "time"

// Account is an external IMAP/SMTP mailbox aggregated into a user's inbox
// (full aggregation client). The password is encrypted at rest with SECRET_KEY;
// the row never exposes it over the wire.
type Account struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserEmail    string    `gorm:"size:255;not null;index" json:"user_email"` // owning mailess account
	Name         string    `gorm:"size:255;not null" json:"name"`             // display label, e.g. "Gmail"
	Email        string    `gorm:"size:255;not null" json:"email"`            // the external address
	ImapHost     string    `gorm:"size:255;not null" json:"imap_host"`
	ImapPort     int       `gorm:"not null;default:993" json:"imap_port"`
	ImapSecurity string    `gorm:"size:16;not null;default:tls" json:"imap_security"` // none | starttls | tls
	SmtpHost     string    `gorm:"size:255" json:"smtp_host"`
	SmtpPort     int       `gorm:"not null;default:465" json:"smtp_port"`
	SmtpSecurity string    `gorm:"size:16;not null;default:tls" json:"smtp_security"` // none | starttls | tls
	Username     string    `gorm:"size:255;not null" json:"username"`
	PasswordEnc  string    `gorm:"type:text;not null" json:"-"`
	Enabled      bool      `gorm:"not null;default:true" json:"enabled"`
	LastError    string    `gorm:"type:text" json:"last_error"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
