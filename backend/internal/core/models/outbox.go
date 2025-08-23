package models

import "time"

// Outbox is a send-undo queue entry: a fully built message parked for a few
// seconds before submission so the sender can cancel it (undo send). The
// worker delivers due entries through the local MTA; persistence makes the
// window survive a backend restart.
type Outbox struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AccountEmail string    `gorm:"index;size:255" json:"account_email"`
	AccountID    uint      `gorm:"index;default:0" json:"-"` // 0 = internal; external aggregated account row
	FromAddr     string    `gorm:"size:255" json:"from_addr"`
	Subject      string    `gorm:"size:255" json:"subject"`
	Recipients   string    `gorm:"type:text" json:"-"` // comma-separated envelope rcpt (to+cc+bcc)
	RawMessage   string    `gorm:"type:text" json:"-"`
	SendAfter    time.Time `gorm:"index" json:"send_after"`
	Status       string    `gorm:"size:16;index;default:pending" json:"status"` // pending|sent|failed|cancelled
	Error        string    `gorm:"size:255" json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
