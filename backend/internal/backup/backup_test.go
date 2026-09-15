package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "control.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.BackupRun{}, &models.SystemFlag{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	uploads := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(filepath.Join(uploads, "sub"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "sub", "nested.bin"), make([]byte, 3*chunkSize/2), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := core.Config{
		DBDriver:  "sqlite",
		DBDSN:     dsn,
		UploadDir: uploads,
		BackupKey: "test-passphrase",
	}
	return &Engine{DB: db, Cfg: cfg}
}

func localTarget(t *testing.T, e *Engine) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "backups")
	e.Cfg.BackupTarget = "local:" + dir
	e.Cfg.BackupKeep = 14
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

// A full run publishes one encrypted archive that verifies, and the run
// history records the success.
func TestRunNowArchiveAndVerify(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)
	target := localTarget(t, e)

	if err := e.RunNow(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 {
		t.Fatalf("target contents: %v %v", entries, err)
	}
	if !strings.HasSuffix(entries[0].Name(), archiveSuffix) {
		t.Fatalf("published name wrong: %s", entries[0].Name())
	}

	var rec models.BackupRun
	if err := e.DB.First(&rec).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if !rec.OK || rec.Size == 0 || rec.FinishedAt == nil {
		t.Fatalf("run not recorded as ok: %+v", rec)
	}
	n, err := e.Verify(ctx, rec.ID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	// control.db + hello.txt + nested.bin
	if n != 3 {
		t.Fatalf("verified %d entries, want 3", n)
	}
}

// A flipped byte must fail decryption, not silently produce garbage.
func TestVerifyDetectsTampering(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)
	target := localTarget(t, e)

	if err := e.RunNow(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	var rec models.BackupRun
	e.DB.First(&rec)

	archive := filepath.Join(target, rec.Detail)
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF
	if err := os.WriteFile(archive, raw, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if _, err := e.Verify(ctx, rec.ID); err == nil {
		t.Fatal("tampered archive verified")
	}
}

// A wrong passphrase fails verification.
func TestVerifyWrongKey(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)
	localTarget(t, e)

	if err := e.RunNow(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	var rec models.BackupRun
	e.DB.First(&rec)
	e.Cfg.BackupKey = "other-passphrase"
	if _, err := e.Verify(ctx, rec.ID); err == nil {
		t.Fatal("wrong key verified")
	}
}

// Pruning keeps only the newest BackupKeep archives.
func TestPrune(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)
	target := localTarget(t, e)
	e.Cfg.BackupKeep = 2

	for i := 0; i < 4; i++ {
		if err := e.RunNow(ctx); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		// Timestamped names have one-second granularity; force distinct
		// names so ordering is deterministic.
		time.Sleep(1100 * time.Millisecond)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("prune left %d archives, want 2", len(entries))
	}
}

// An empty key must refuse to archive rather than write plaintext.
func TestRefusesWithoutKey(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)
	localTarget(t, e)
	e.Cfg.BackupKey = ""

	if err := e.RunNow(ctx); err == nil {
		t.Fatal("ran without an encryption key")
	}
	var count int64
	e.DB.Model(&models.BackupRun{}).Count(&count)
	if count != 1 {
		t.Fatalf("refusal should still record the run, got %d rows", count)
	}
}

// Unsupported control-plane drivers fail with guidance instead of a broken
// archive.
func TestRefusesNonSQLite(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)
	localTarget(t, e)
	e.Cfg.DBDriver = "mysql"

	if err := e.RunNow(ctx); err == nil {
		t.Fatal("non-sqlite driver should refuse")
	}
}
