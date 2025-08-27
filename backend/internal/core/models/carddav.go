package models

import "time"

// CardDAVConfig stores the user's remote CardDAV address book endpoint used
// for one-way import sync into the local contacts. The password is encrypted
// at rest.
type CardDAVConfig struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserEmail   string    `gorm:"size:255;not null;uniqueIndex" json:"user_email"`
	URL         string    `gorm:"size:1024;not null" json:"url"`
	Username    string    `gorm:"size:255;not null;default:''" json:"username"`
	PasswordEnc string    `gorm:"size:1024;not null;default:''" json:"-"`
	UpdatedAt   time.Time `json:"updated_at"`
}
