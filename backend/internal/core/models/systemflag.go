package models

import "time"

// SystemFlag is a tiny persistent key/value store for worker bookkeeping
// (e.g. the digest worker's last-send date). Values are opaque strings.
type SystemFlag struct {
	Key       string    `gorm:"primaryKey;size:80" json:"key"`
	Value     string    `gorm:"size:255" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TrafficPoint is one scrape of the engine's Prometheus counters. Counters
// are cumulative per engine process; day deltas (with restart detection)
// are computed at read time.
type TrafficPoint struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TS          time.Time `gorm:"index" json:"ts"`
	InAccepted  int64     `json:"in_accepted"`
	InRejected  int64     `json:"in_rejected"`
	InDeferred  int64     `json:"in_deferred"`
	OutDelivered int64    `json:"out_delivered"`
	OutBounced  int64     `json:"out_bounced"`
	OutDeferred int64     `json:"out_deferred"`
	QueueDepth  int64     `json:"queue_depth"`
}
