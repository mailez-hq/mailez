package models

import "time"

// PGPKey is an imported public key in a user's personal keyring. It lets the
// composer encrypt outgoing mail to addresses outside the local users table.
type PGPKey struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserEmail   string    `gorm:"size:255;not null;index" json:"user_email"`
	Email       string    `gorm:"size:255;not null;index" json:"email"`
	PublicKey   string    `gorm:"type:text;not null" json:"public_key"`
	Fingerprint string    `gorm:"size:64;not null" json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}
