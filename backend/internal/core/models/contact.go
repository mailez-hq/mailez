package models

import "time"

// Contact is a personal address-book entry owned by a user.
type Contact struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserEmail string    `gorm:"size:255;not null;index" json:"user_email"`
	Name      string    `gorm:"size:160;not null" json:"name"`
	Email     string    `gorm:"size:255;not null" json:"email"`
	Comment   string    `gorm:"size:255;default:''" json:"comment"`
	Groups    string    `gorm:"size:255;default:''" json:"groups"`  // comma-separated group names
	Avatar    string    `gorm:"size:1024;default:''" json:"avatar"` // URL or data URI
	CreatedAt time.Time `json:"created_at"`
}
