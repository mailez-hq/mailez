package models

import "time"

// AuditLog is one administrative action recorded for accountability.
// Action/Target/Detail are a semantic enrichment of the raw method/path
// record; deployments without the enrichment leave them empty (see core
// auditEnrich).
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	User      string    `gorm:"size:255;not null;index" json:"user"`
	IP        string    `gorm:"size:64" json:"ip"`
	Method    string    `gorm:"size:8;not null" json:"method"`
	Path      string    `gorm:"size:255;not null" json:"path"`
	Status    int       `gorm:"not null" json:"status"`
	Action    string    `gorm:"size:64;index" json:"action,omitempty"`
	Target    string    `gorm:"size:255" json:"target,omitempty"`
	Detail    string    `gorm:"size:512" json:"detail,omitempty"`
}
