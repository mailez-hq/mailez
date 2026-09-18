package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

func workerTokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "worker.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.Token{}, &models.WorkerToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// Re-minting must keep the row's created_at: Save() writes every field, and a
// fresh struct sends the zero time as '0000-00-00', which MySQL's strict mode
// rejects — the update never landed, the new secret was never persisted, and
// every retry minted another Token row and failed again (SSE and the outbox
// worker were dead for that user). SQLite accepts the zero date, so the
// assertion here is on the row, not on an error.
func TestEnsureWorkerTokenKeepsCreatedAt(t *testing.T) {
	db := workerTokenDB(t)
	ctx := context.Background()

	first, err := EnsureWorkerToken(ctx, db, "test-secret", "a@example.com")
	if err != nil {
		t.Fatalf("first mint: %v", err)
	}
	var row models.WorkerToken
	if err := db.First(&row, "user_email = ?", "a@example.com").Error; err != nil {
		t.Fatalf("load worker token: %v", err)
	}
	if row.CreatedAt.IsZero() {
		t.Fatal("created_at must be set on the first mint")
	}
	created := row.CreatedAt

	// The same call again reuses the stored secret: no new Token row.
	again, err := EnsureWorkerToken(ctx, db, "test-secret", "a@example.com")
	if err != nil || again != first {
		t.Fatalf("second call = %q, %v, want the stored secret", again, err)
	}
	var count int64
	if err := db.Model(&models.Token{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("token rows = %d, want 1 (reused, not re-minted)", count)
	}

	// Revoke the credential the stored secret points at, which forces a
	// re-mint: the row must survive with its original created_at, and the
	// freshly minted secret must be what the next call returns.
	if err := db.Where("user_email = ?", "a@example.com").Delete(&models.Token{}).Error; err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	third, err := EnsureWorkerToken(ctx, db, "test-secret", "a@example.com")
	if err != nil {
		t.Fatalf("re-mint: %v", err)
	}
	if third == first {
		t.Fatal("a revoked credential must be replaced, not reused")
	}
	var after models.WorkerToken
	if err := db.First(&after, "user_email = ?", "a@example.com").Error; err != nil {
		t.Fatalf("reload worker token: %v", err)
	}
	if !after.CreatedAt.Equal(created) {
		t.Fatalf("created_at = %v, want it preserved as %v", after.CreatedAt, created)
	}
	if after.TokenEnc == row.TokenEnc {
		t.Fatal("the stored secret must be replaced on a re-mint")
	}
}
