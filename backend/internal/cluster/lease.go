// Package cluster provides the coordination primitives the control plane
// needs to run several stateless replicas against one shared database.
//
// The only primitive is a DB-backed lease: singleton workers (fetch
// polling, push notifications, retention sweeps, reminders...) call
// TryHold once per tick and skip when it returns false, so exactly one
// replica runs each loop. Leases expire, so when the leader dies another
// replica takes over within one TTL without operator action. On the
// single-replica SQLite default every TryHold trivially succeeds and the
// guard is free.
package cluster

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mailez/backend/internal/core/models"
)

// LeaseTTL bounds failover time: a dead leader's leases free up at most
// this long after its last heartbeat. Workers renew on every tick, and all
// stock worker ticks are well under a minute, so a minute is generous.
const LeaseTTL = time.Minute

var (
	holderOnce sync.Once
	holder     string
)

// holderID identifies this replica for lease ownership: hostname, pid and
// a random suffix, so a restarted process cannot inherit its own expired
// lease by coincidence while another replica is mid-takeover.
func holderID() string {
	holderOnce.Do(func() {
		host, _ := os.Hostname()
		var suffix [4]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			suffix = [4]byte{0, 0, 0, 0}
		}
		holder = fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(suffix[:]))
	})
	return holder
}

// TryHold reports whether this replica holds the named lease, acquiring or
// renewing it otherwise. Callers must treat a false return as "another
// replica owns this worker right now" and skip the tick.
//
// The acquire path is a single INSERT ... ON CONFLICT/IGNORE so the common
// case is one round trip; the renew path is a guarded UPDATE that only
// succeeds for the current holder or after expiry. Concurrent takeovers
// serialize on the row lock: exactly one UPDATE reports rows affected.
// Expiry compares replica clocks, so replicas must run NTP (standard for
// any multi-replica deployment anyway).
func TryHold(db *gorm.DB, name string, ttl time.Duration) bool {
	if db == nil {
		// No database (should not happen): fail open so workers keep
		// running in single-replica deployments.
		return true
	}
	now := time.Now().UTC()
	lease := models.ClusterLease{Name: name, Holder: holderID(), ExpiresAt: now.Add(ttl)}
	// First acquire: inserting wins if the row does not exist yet. A
	// missing table (deployment that skipped migrations, or a bare test
	// database) self-heals once here instead of silently disabling the
	// worker.
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease)
	if res.Error != nil {
		if err := db.AutoMigrate(&models.ClusterLease{}); err != nil {
			return false
		}
		res = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease)
	}
	if res.Error == nil {
		if res.RowsAffected == 1 {
			return true
		}
		// Row exists: take it over when free (expired) or renew when ours.
		res = db.Model(&models.ClusterLease{}).
			Where("name = ? AND (holder = ? OR expires_at <= ?)", name, holderID(), now).
			Updates(map[string]any{"holder": holderID(), "expires_at": now.Add(ttl)})
		return res.Error == nil && res.RowsAffected > 0
	}
	return false
}
