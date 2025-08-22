package models

import "time"

// Fetch is a remote POP/IMAP account fetched into a local account.
type Fetch struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	UserEmail string `gorm:"size:255;not null" json:"user_email"`
	Protocol  string `gorm:"size:16;not null" json:"protocol"`
	Host      string `gorm:"size:255;not null" json:"host"`
	Port      int    `gorm:"not null" json:"port"`
	TLS       bool   `gorm:"not null;default:false" json:"tls"`
	Username  string `gorm:"size:255;not null" json:"username"`
	Password  string `gorm:"size:255;not null" json:"-"`
	Keep      bool   `gorm:"not null;default:false" json:"keep"`
	Scan      bool   `gorm:"not null;default:false" json:"scan"`
	Invisible bool   `gorm:"not null;default:false" json:"invisible"`
	Folders   string `gorm:"size:4096" json:"folders"`
	LastCheck *time.Time `json:"last_check"`
	Error     string `gorm:"size:1023" json:"error"`
}
