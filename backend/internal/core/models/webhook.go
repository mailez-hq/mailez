package models

import "time"

// Webhook is a user-configured HTTP endpoint that receives event callbacks
// (new mail, etc.). Each delivery is signed with the user-chosen secret via an
// X-Mailez-Signature header so the receiver can verify authenticity.
type Webhook struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserEmail  string     `gorm:"size:255;not null;index" json:"user_email"`
	URL        string     `gorm:"type:text;not null" json:"url"`
	Secret     string     `gorm:"type:text;not null" json:"secret"`
	Events     string     `gorm:"type:text;not null" json:"events"` // comma-separated: mail.received
	Enabled    bool       `gorm:"not null;default:true" json:"enabled"`
	// TokenEnc/TokenID back the background notifier poller for users without
	// a browser push subscription (webhook-only mail.received delivery).
	TokenEnc   string     `gorm:"type:text" json:"-"`
	TokenID    uint       `json:"-"`
	LastStatus int        `json:"last_status"` // 0 = never fired yet
	LastError  string     `gorm:"type:text" json:"last_error"`
	LastSentAt *time.Time `json:"last_sent_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
