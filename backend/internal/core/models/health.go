package models

import "time"

// HealthSnapshot is the health alerter's per-probe memory: the last
// announced status, when it became active, and a candidate status observed
// once but not yet confirmed by the next run (flap suppression).
type HealthSnapshot struct {
	Key          string    `gorm:"primaryKey;size:190" json:"key"`
	Status       string    `gorm:"size:16" json:"status"`
	Since        time.Time `json:"since"`
	Pending      string    `gorm:"size:16" json:"pending,omitempty"`
	PendingSince time.Time `json:"pending_since,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}
