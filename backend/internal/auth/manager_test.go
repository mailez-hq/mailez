package auth

import (
	"context"
	"path/filepath"
	"strings"
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

func TestSessionTokenStablePerSession(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()
	sid, user, err := mgr.Login(ctx, "a@example.com", "correct-horse")
	if err != nil || user == nil || sid == "" {
		t.Fatalf("login: sid=%q user=%v err=%v", sid, user, err)
	}

	tok1, err := mgr.SessionToken(ctx, "a@example.com", sid)
	if err != nil {
		t.Fatalf("session token: %v", err)
	}
	if !strings.HasPrefix(tok1, "token-") {
		t.Fatalf("token must carry the token- prefix, got %q", tok1)
	}
	if !mgr.VerifyTempToken(ctx, "a@example.com", tok1) {
		t.Fatal("fresh session token must verify")
	}

	tok2, err := mgr.SessionToken(ctx, "a@example.com", sid)
	if err != nil {
		t.Fatalf("session token (2nd): %v", err)
	}
	if tok1 != tok2 {
		t.Fatalf("session token must be stable, got %q then %q", tok1, tok2)
	}
}

func TestSessionTokenDistinctSessions(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()
	sid1, _, err := mgr.Login(ctx, "a@example.com", "correct-horse")
	if err != nil {
		t.Fatalf("login 1: %v", err)
	}
	sid2, _, err := mgr.Login(ctx, "a@example.com", "correct-horse")
	if err != nil {
		t.Fatalf("login 2: %v", err)
	}
	tok1, _ := mgr.SessionToken(ctx, "a@example.com", sid1)
	tok2, _ := mgr.SessionToken(ctx, "a@example.com", sid2)
	if tok1 == "" || tok2 == "" || tok1 == tok2 {
		t.Fatalf("different sessions must get different tokens: %q vs %q", tok1, tok2)
	}
}

func TestSessionTokenInvalidatedByLogout(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()
	sid, _, err := mgr.Login(ctx, "a@example.com", "correct-horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	tok, _ := mgr.SessionToken(ctx, "a@example.com", sid)
	if !mgr.VerifyTempToken(ctx, "a@example.com", tok) {
		t.Fatal("token must verify before logout")
	}
	if err := mgr.Logout(ctx, sid); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if mgr.VerifyTempToken(ctx, "a@example.com", tok) {
		t.Fatal("token must not verify after logout")
	}
}

func TestSessionTokenAfterStoreEvictionMintsFresh(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()
	sid, _, err := mgr.Login(ctx, "a@example.com", "correct-horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	tok1, _ := mgr.SessionToken(ctx, "a@example.com", sid)

	// Simulate the store evicting the token row while the session-token
	// mapping survives: the next call must mint a fresh valid token instead
	// of replaying the orphaned one.
	if err := mgr.Store.Delete(ctx, tokenKeyPrefix+tok1); err != nil {
		t.Fatalf("delete token: %v", err)
	}
	tok2, err := mgr.SessionToken(ctx, "a@example.com", sid)
	if err != nil {
		t.Fatalf("session token after eviction: %v", err)
	}
	if tok1 == tok2 {
		t.Fatal("orphaned mapping must not be replayed")
	}
	if !mgr.VerifyTempToken(ctx, "a@example.com", tok2) {
		t.Fatal("fresh token must verify")
	}
}
