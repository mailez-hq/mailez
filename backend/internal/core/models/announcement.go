package models

import "time"

// Announcement is a single global notice shown to every user in the webmail
// (Mailu parity). Publishing a new one replaces the previous; clearing it
// removes the banner everywhere.
type Announcement struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Subject   string    `gorm:"size:255;not null" json:"subject"`
	Body      string    `gorm:"type:text;not null" json:"body"`
	Enabled   bool      `gorm:"not null;default:false" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
