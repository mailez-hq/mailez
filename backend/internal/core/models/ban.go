package models

import "time"

// BanRecord is one IP ban. Rows are created by the ban engine when
// authentication failures from one source IP cross the configured threshold
// within its window, and stay as history after expiry or a manual lift.
type BanRecord struct {
	ID uint   `gorm:"primaryKey" json:"id"`
	IP string `gorm:"size:64;index" json:"ip"`
	// Surface is the auth path that triggered the ban
	// ("web", "imap", "pop3", "smtp", "submission", ...).
	Surface string `gorm:"size:16" json:"surface"`
	// Failed is the failure count within the window at trigger time.
	Failed    int       `json:"failed"`
	Until     time.Time `gorm:"index" json:"until"`
	CreatedAt time.Time `json:"created_at"`
	// LiftedAt is set when an administrator lifts the ban early.
	LiftedAt *time.Time `json:"lifted_at,omitempty"`
}
