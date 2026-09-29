package ban

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// memCounter is an in-memory windowed counter mirroring the session store's
// semantics (fixed window anchored at the first increment).
type memCounter struct {
	mu  sync.Mutex
	n   map[string]int
	exp map[string]time.Time
}

func newMemCounter() *memCounter {
	return &memCounter{n: map[string]int{}, exp: map[string]time.Time{}}
}

func (c *memCounter) Incr(_ context.Context, key string, ttl time.Duration) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if exp, ok := c.exp[key]; !ok || time.Now().After(exp) {
		c.n[key] = 0
		c.exp[key] = time.Now().Add(ttl)
	}
	c.n[key]++
	return c.n[key], nil
}

func (c *memCounter) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.n, key)
	delete(c.exp, key)
	return nil
}

func newEngineDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ban.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.BanRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testCfg(whitelist string) core.Config {
	return core.Config{
		BanMaxRetry:    3,
		BanFindTimeSec: 60,
		BanWhitelist:   whitelist,
		Hostname:       "localhost",
	}
}

func TestFailureBansAtThreshold(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	for i := 0; i < 2; i++ {
		e.Failure(ctx, "203.0.113.7", "imap")
	}
	if e.Active(ctx, "203.0.113.7") {
		t.Fatal("banned below the threshold")
	}
	e.Failure(ctx, "203.0.113.7", "imap")
	if !e.Active(ctx, "203.0.113.7") {
		t.Fatal("not banned at the threshold")
	}
	records, err := e.List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list: %v %v", records, err)
	}
	if records[0].Surface != "imap" || records[0].Failed != 3 {
		t.Fatalf("record wrong: %+v", records[0])
	}
}

func TestWhitelistExempt(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg("10.0.0.0/8"))

	for i := 0; i < 10; i++ {
		e.Failure(ctx, "10.1.2.3", "web")
	}
	if e.Active(ctx, "10.1.2.3") {
		t.Fatal("whitelisted IP got banned")
	}
}

func TestResetClearsWindow(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	e.Failure(ctx, "203.0.113.9", "web")
	e.Failure(ctx, "203.0.113.9", "web")
	e.Reset(ctx, "203.0.113.9")
	for i := 0; i < 2; i++ {
		e.Failure(ctx, "203.0.113.9", "web")
	}
	if e.Active(ctx, "203.0.113.9") {
		t.Fatal("window not cleared by Reset")
	}
}

// A repeat offender's second ban outlasts the first.
func TestEscalation(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	for i := 0; i < 3; i++ {
		e.Failure(ctx, "203.0.113.10", "web")
	}
	e.Lift(ctx, 1)
	// The first ban stays as history; the counter restarts after Lift.
	for i := 0; i < 3; i++ {
		e.Failure(ctx, "203.0.113.10", "web")
	}
	records, err := e.List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list: %v %v", records, err)
	}
	var first, second models.BanRecord
	if err := e.DB.First(&first, 1).Error; err != nil {
		t.Fatalf("first ban: %v", err)
	}
	if err := e.DB.First(&second, 2).Error; err != nil {
		t.Fatalf("second ban: %v", err)
	}
	if second.Until.Sub(second.CreatedAt) <= first.Until.Sub(first.CreatedAt) {
		t.Fatalf("second ban not escalated: %v vs %v",
			first.Until.Sub(first.CreatedAt), second.Until.Sub(second.CreatedAt))
	}
}

func TestLift(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	for i := 0; i < 3; i++ {
		e.Failure(ctx, "203.0.113.11", "web")
	}
	if !e.Active(ctx, "203.0.113.11") {
		t.Fatal("not banned before lift")
	}
	if err := e.Lift(ctx, 1); err != nil {
		t.Fatalf("lift: %v", err)
	}
	if e.Active(ctx, "203.0.113.11") {
		t.Fatal("still banned after lift")
	}
}

func TestRecentCount(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	for i := 0; i < 3; i++ {
		e.Failure(ctx, "203.0.113.12", "web")
	}
	n, err := e.RecentCount(ctx, time.Now().Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("recent count: %d %v", n, err)
	}
}

// An auth attempt with no resolvable source must never be banned, and must
// never create a ban row. Rows keyed on "" compare equal to every other
// unattributed request, so one such row locks out every unattributed login,
// including the mail engine's own service-to-service calls.
func TestUnattributableNeverBans(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	for i := 0; i < 10; i++ {
		e.Failure(ctx, "", "imap")
	}

	var n int64
	if err := e.DB.Model(&models.BanRecord{}).Where("ip = ?", "").Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("empty source IP created %d ban row(s)", n)
	}
	if e.Active(ctx, "") {
		t.Fatal("empty source IP reported as banned")
	}
}

// Same for a value that is present but not an IP.
func TestMalformedSourceNeverBans(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	for _, ip := range []string{"not-an-ip", "999.999.999.999", " "} {
		for i := 0; i < 10; i++ {
			e.Failure(ctx, ip, "imap")
		}
		if e.Active(ctx, ip) {
			t.Fatalf("%q reported as banned", ip)
		}
	}

	var n int64
	if err := e.DB.Model(&models.BanRecord{}).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("malformed source IPs created %d ban row(s)", n)
	}
}

// A stale row already keyed on "" -- written before the guard existed, or by
// an older build -- must not lock out an unattributed login.
func TestPreExistingEmptyRowDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	e := New(newEngineDB(t), newMemCounter(), testCfg(""))

	if err := e.DB.Create(&models.BanRecord{
		IP:      "",
		Surface: "smtp",
		Failed:  20,
		Until:   time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}

	if e.Active(ctx, "") {
		t.Fatal("legacy empty-IP row still gates unattributed logins")
	}
	// A real attacker is still gated.
	if err := e.DB.Create(&models.BanRecord{
		IP:      "203.0.113.20",
		Surface: "smtp",
		Failed:  20,
		Until:   time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if !e.Active(ctx, "203.0.113.20") {
		t.Fatal("attributable ban no longer enforced")
	}
}
