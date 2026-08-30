package models

import "time"

// ClusterLease is a DB-backed singleton-worker lock. Control-plane
// replicas acquire short leases by worker name (fetch, push notifier,
// retention sweeps, ...) so exactly one replica runs each periodic loop;
// expired rows free up on their own, which makes failover automatic.
type ClusterLease struct {
	Name      string    `gorm:"primaryKey;size:64" json:"name"`
	Holder    string    `gorm:"size:128" json:"holder"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
}
