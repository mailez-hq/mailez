package models

import "time"

// SmimeCert is an imported S/MIME certificate used to encrypt outgoing mail
// to an external recipient. Unlike the user's own certificate (kept on the
// User row), these only carry the public half.
type SmimeCert struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserEmail   string    `gorm:"size:255;not null;index" json:"user_email"`
	Email       string    `gorm:"size:255;not null;index" json:"email"`
	CertPEM     string    `gorm:"type:text;not null" json:"cert_pem"`
	Fingerprint string    `gorm:"size:64;not null" json:"fingerprint"`
	Subject     string    `gorm:"size:512" json:"subject"`
	Issuer      string    `gorm:"size:512" json:"issuer"`
	NotAfter    time.Time `json:"not_after"`
	CreatedAt   time.Time `json:"created_at"`
}
