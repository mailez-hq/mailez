package models

import "time"

// Label is a user-defined tag applied to messages as an IMAP keyword. The row
// itself only carries presentation data (color) so labels stay listed in the
// sidebar even when no currently loaded message carries the keyword.
type Label struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserEmail string    `gorm:"size:255;not null;uniqueIndex:uk_labels_user_name" json:"user_email"`
	Name      string    `gorm:"size:64;not null;uniqueIndex:uk_labels_user_name" json:"name"`
	Color     string    `gorm:"size:16;not null;default:''" json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
