package models

import "time"

// BackupRun is one scheduled or manual encrypted-backup attempt, kept as
// the run history the admin console and the freshness probe report from.
type BackupRun struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// StartedAt/FinishedAt bracket the whole archive-and-publish step.
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// OK marks a run whose archive reached the target and passed pruning.
	OK bool `json:"ok"`
	// Size is the published archive size in bytes (0 when the run failed).
	Size int64 `json:"size"`
	// Target is the configured destination the run published to.
	Target string `gorm:"size:255" json:"target"`
	// Detail carries the published archive name, or the failure reason.
	Detail string `gorm:"size:512" json:"detail"`
}
