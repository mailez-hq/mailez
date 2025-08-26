package models

import "time"

// Label is a user-defined tag applied to messages as an IMAP keyword. The row
// carries the presentation data (color) plus the ASCII IMAP keyword that the
// display name maps to; non-ASCII names (e.g. Chinese) are =XX-encoded so the
// wire stays a valid atom while the UI keeps showing the display name.
type Label struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserEmail string    `gorm:"size:255;not null;uniqueIndex:uk_labels_user_name" json:"user_email"`
	Name      string    `gorm:"size:64;not null;uniqueIndex:uk_labels_user_name" json:"name"`
	Keyword   string    `gorm:"size:64;not null;default:''" json:"keyword"`
	Color     string    `gorm:"size:16;not null;default:''" json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
