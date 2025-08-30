package cluster

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

func newLeaseDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lease.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.ClusterLease{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// Two replicas contending for one lease: exactly one holds it, renewals
// stay with the holder, and the loser keeps losing while it is held.
func TestTryHoldExclusive(t *testing.T) {
	db := newLeaseDB(t)
	if !TryHoldWith(db, "worker", time.Minute, "replica-a") {
		t.Fatal("first acquire must win")
	}
	if TryHoldWith(db, "worker", time.Minute, "replica-b") {
		t.Fatal("second replica must not win a held lease")
	}
	if !TryHoldWith(db, "worker", time.Minute, "replica-a") {
		t.Fatal("holder must be able to renew")
	}
	if TryHoldWith(db, "worker", time.Minute, "replica-b") {
		t.Fatal("lease must still exclude the other replica after renewal")
	}
}

// A dead leader's expired lease is taken over by the next replica, which
// then excludes both the old holder and further contenders.
func TestTryHoldTakeoverAfterExpiry(t *testing.T) {
	db := newLeaseDB(t)
	if !TryHoldWith(db, "worker", time.Minute, "replica-a") {
		t.Fatal("first acquire must win")
	}
	// Simulate leader death: the lease stops being renewed and time passes
	// beyond its TTL.
	if err := db.Model(&models.ClusterLease{}).Where("name = ?", "worker").
		Update("expires_at", time.Now().UTC().Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if !TryHoldWith(db, "worker", time.Minute, "replica-b") {
		t.Fatal("replica-b must take over an expired lease")
	}
	if TryHoldWith(db, "worker", time.Minute, "replica-a") {
		t.Fatal("the dead leader must not reacquire a live lease")
	}
	if TryHoldWith(db, "worker", time.Minute, "replica-c") {
		t.Fatal("the new holder must exclude further contenders")
	}
}

// Under contention, a held lease lets exactly one replica through per
// decision point and no other replica ever sneaks in.
func TestTryHoldConcurrentExclusivity(t *testing.T) {
	db := newLeaseDB(t)
	if !TryHoldWith(db, "worker", 10*time.Second, "replica-a") {
		t.Fatal("seed acquire must win")
	}
	var wins int32
	const replicas = 8
	const rounds = 50
	var wg sync.WaitGroup
	for r := 0; r < replicas; r++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			holder := "challenger-" + string(rune('a'+i))
			for j := 0; j < rounds; j++ {
				if TryHoldWith(db, "worker", 10*time.Second, holder) {
					atomic.AddInt32(&wins, 1)
				}
			}
		}(r)
	}
	wg.Wait()
	if wins != 0 {
		t.Fatalf("challengers won %d times against a live lease, want 0", wins)
	}
}
