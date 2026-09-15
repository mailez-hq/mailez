// Integration tests for the health-alert worker's flush loop against a
// real database and a real webhook receiver: baseline on first sight, the
// two-run confirmation before announcing, recovery notices, and muting.
package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

type webhookCapture struct {
	mu     sync.Mutex
	calls  int
	bodies []string
}

func (c *webhookCapture) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.calls++
		c.bodies = append(c.bodies, string(b))
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
}

func (c *webhookCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *webhookCapture) saw(substr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range c.bodies {
		if strings.Contains(b, substr) {
			return true
		}
	}
	return false
}

func newWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alert.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// degrade rewrites the last announced state of a probe as failing, so the
// next flush observes a recovery candidate.
func degrade(t *testing.T, db *gorm.DB, key string) {
	t.Helper()
	res := db.Model(&models.HealthSnapshot{}).
		Where(&models.HealthSnapshot{Key: key}).
		Updates(map[string]any{"status": "fail", "pending": "", "pending_since": nil})
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("seed degraded snapshot %s: rows=%d err=%v", key, res.RowsAffected, res.Error)
	}
}

func snapshotOf(t *testing.T, db *gorm.DB, key string) models.HealthSnapshot {
	t.Helper()
	var snap models.HealthSnapshot
	if err := db.Where(&models.HealthSnapshot{Key: key}).First(&snap).Error; err != nil {
		t.Fatalf("load snapshot %s: %v", key, err)
	}
	return snap
}

// The full transition story: first flush is a silent baseline; a status
// change must hold for two consecutive flushes before the webhook fires;
// the announcement flips the stored state.
func TestHealthAlertWorkerAnnouncesAfterConfirmation(t *testing.T) {
	capture := &webhookCapture{}
	ts := capture.server()
	defer ts.Close()

	db := newWorkerDB(t)
	w := &HealthAlertWorker{DB: db, Cfg: core.Config{HealthAlertWebhook: ts.URL}}

	w.flush()
	if capture.count() != 0 {
		t.Fatal("baseline flush must not announce")
	}
	var baseline int64
	db.Model(&models.HealthSnapshot{}).Count(&baseline)
	if baseline == 0 {
		t.Fatal("baseline flush recorded no snapshots")
	}
	snap := snapshotOf(t, db, "system:database")
	if snap.Status != statusOK {
		t.Fatalf("system:database baseline = %q, want ok", snap.Status)
	}

	degrade(t, db, "system:database")
	w.flush()
	snap = snapshotOf(t, db, "system:database")
	if snap.Status != statusFail || snap.Pending != statusOK {
		t.Fatalf("after first observed recovery: status=%q pending=%q, want fail/ok", snap.Status, snap.Pending)
	}
	if capture.count() != 0 {
		t.Fatal("one-run candidate must not announce")
	}

	w.flush()
	snap = snapshotOf(t, db, "system:database")
	if snap.Status != statusOK || snap.Pending != "" {
		t.Fatalf("after confirmation: status=%q pending=%q, want ok/empty", snap.Status, snap.Pending)
	}
	if !capture.saw("system:database") || !capture.saw(`"from":"fail"`) || !capture.saw(`"to":"ok"`) {
		t.Fatalf("webhook missed the recovery event: %v", capture.bodies)
	}
}

// Muted probes are skipped entirely: no state change, no announcement.
// The muted key has a stored snapshot from before it was muted (as in a
// deployment where the operator silences a noisy probe).
func TestHealthAlertWorkerMutesProbe(t *testing.T) {
	capture := &webhookCapture{}
	ts := capture.server()
	defer ts.Close()

	db := newWorkerDB(t)
	w := &HealthAlertWorker{DB: db, Cfg: core.Config{
		HealthAlertWebhook: ts.URL,
		HealthAlertMute:    "system:database",
	}}

	now := time.Now().UTC()
	if err := db.Create(&models.HealthSnapshot{
		Key: "system:database", Status: statusOK, Since: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	degrade(t, db, "system:database")
	w.flush()
	w.flush()

	snap := snapshotOf(t, db, "system:database")
	if snap.Status != statusFail || snap.Pending != "" {
		t.Fatalf("muted probe changed state: status=%q pending=%q", snap.Status, snap.Pending)
	}
	if capture.saw("system:database") {
		t.Fatalf("muted probe announced: %v", capture.bodies)
	}
}

// A muted worker never ticks: Run returns as soon as its context ends.
func TestHealthAlertWorkerDisabled(t *testing.T) {
	db := newWorkerDB(t)
	w := &HealthAlertWorker{DB: db, Cfg: core.Config{HealthAlert: "off"}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disabled worker kept running")
	}
}
