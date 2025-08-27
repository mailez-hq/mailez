package models

import "time"

// Template is a reusable compose snippet (canned response / 常用语) saved
// per account and inserted into the editor with one click.
type Template struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserEmail string    `gorm:"size:255;not null;index" json:"user_email"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Subject   string    `gorm:"size:255;not null;default:''" json:"subject"`
	HTML      string    `gorm:"type:text;not null" json:"html"`
	Text      string    `gorm:"type:text;not null" json:"text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
