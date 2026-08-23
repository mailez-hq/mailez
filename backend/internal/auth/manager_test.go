package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

func newTestManager(t *testing.T) (*Manager, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{
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
	hash, _ := password.Hash("correct-horse")
	if err := db.Create(&models.User{Email: "a@example.com", Localpart: "a", DomainName: "example.com", Password: hash, Enabled: true}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mgr := NewManager(db, NewMemoryStore(), "mailez_session", time.Hour)
	return mgr, db
}

func TestLoginAndSession(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()
	sid, user, err := mgr.Login(ctx, "a@example.com", "correct-horse")
	if err != nil || user == nil || sid == "" {
		t.Fatalf("login: sid=%q user=%v err=%v", sid, user, err)
	}
	got, err := mgr.UserFromSession(ctx, sid)
	if err != nil || got == nil || got.Email != "a@example.com" {
		t.Fatalf("UserFromSession: %v %v", got, err)
	}
	if _, u, _ := mgr.Login(ctx, "a@example.com", "wrong"); u != nil {
		t.Fatal("wrong password must not authenticate")
	}
	if _, err := mgr.UserFromSession(ctx, "no-such-session"); err != nil {
		t.Fatalf("unknown session: %v", err)
	}
}

func TestPending2FASingleUse(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()
	tok, err := mgr.CreatePending2FA(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	email, ok := mgr.ConsumePending2FA(ctx, tok)
	if !ok || email != "a@example.com" {
		t.Fatalf("consume: %q %v", email, ok)
	}
	if _, ok := mgr.ConsumePending2FA(ctx, tok); ok {
		t.Fatal("pending token must be single use")
	}
}

func TestLoginLimiter(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.SetLoginLimits(2, 10)
	ctx := context.Background()
	if !mgr.checkLoginAttempt(ctx, "10.0.0.1") || !mgr.checkLoginAttempt(ctx, "10.0.0.1") {
		t.Fatal("first two attempts must pass")
	}
	if mgr.checkLoginAttempt(ctx, "10.0.0.1") {
		t.Fatal("third attempt must be limited")
	}
	for i := 0; i < 10; i++ {
		if mgr.loginFailed(ctx, "a@example.com") {
			t.Fatal("below the failure threshold nothing is locked out")
		}
	}
	mgr.SetLoginLimits(30, 1)
	if !mgr.loginFailed(ctx, "a@example.com") {
		t.Fatal("exceeding the failure limit must lock the account")
	}
	mgr.SetLoginLimits(30, 10)
	mgr.loginSucceeded(ctx, "a@example.com")
	if mgr.loginFailed(ctx, "a@example.com") {
		t.Fatal("counter must reset after a successful login")
	}
}
