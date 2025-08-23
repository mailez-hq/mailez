package models

import "time"

// AuditLog is one administrative action recorded for accountability,
// mirroring the reference implementation's audit facility.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	User      string    `gorm:"size:255;not null" json:"user"`
	IP        string    `gorm:"size:64" json:"ip"`
	Method    string    `gorm:"size:8;not null" json:"method"`
	Path      string    `gorm:"size:255;not null" json:"path"`
	Status    int       `gorm:"not null" json:"status"`
}
