package models

import "time"

// Base holds columns shared by all models.
type Base struct {
	CreatedAt time.Time  `gorm:"type:date;not null" json:"created_at"`
	UpdatedAt *time.Time `gorm:"type:date" json:"updated_at"`
	Comment   string     `gorm:"size:255;default:''" json:"comment"`
}
