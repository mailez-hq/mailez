package webauthn

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

// memStore is an in-memory challenge store for tests.
type memStore struct{ m map[string]string }

func newMemStore() *memStore { return &memStore{m: map[string]string{}} }
func (s *memStore) Set(_ context.Context, k, v string, _ time.Duration) error {
	s.m[k] = v
	return nil
}
func (s *memStore) Get(_ context.Context, k string) (string, bool, error) {
	v, ok := s.m[k]
	return v, ok, nil
}
func (s *memStore) Delete(_ context.Context, k string) error {
	delete(s.m, k)
	return nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "wa.db")), &gorm.Config{
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
	if err := db.Create(&models.User{Email: "a@example.com", Localpart: "a", DomainName: "example.com", Password: "x", Enabled: true}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc, err := New(db, newMemStore(), "example.com", "Mailez", []string{"https://mail.example.com"})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestNewRequiresConfig(t *testing.T) {
	if _, err := New(nil, newMemStore(), "", "Mailez", nil); err == nil {
		t.Fatal("empty rp id accepted")
	}
	if _, err := New(nil, newMemStore(), "example.com", "Mailez", nil); err == nil {
		t.Fatal("empty origins accepted")
	}
}

func TestBeginLoginWithoutCredentials(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.BeginLogin("a@example.com"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if _, err := svc.BeginLogin("nobody@example.com"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unknown account err = %v, want ErrUnavailable", err)
	}
}

func TestBeginRegistrationStoresChallenge(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.BeginRegistration("nobody@example.com"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	options, err := svc.BeginRegistration("a@example.com")
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	if options == nil {
		t.Fatal("options = nil")
	}
	if _, ok := svc.Store.(*memStore); !ok {
		t.Fatal("unexpected store type")
	}
	if len(svc.Store.(*memStore).m) == 0 {
		t.Fatal("challenge not persisted")
	}
}

func TestListAndDeleteScopedToOwner(t *testing.T) {
	svc := newTestService(t)
	row := &models.WebauthnCredential{
		UserEmail:    "a@example.com",
		Name:         "laptop",
		CredentialID: "cred-1",
		PublicKey:    []byte{1, 2, 3},
	}
	if err := svc.DB.Create(row).Error; err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	if err := svc.DB.Create(&models.WebauthnCredential{UserEmail: "other@example.com", CredentialID: "cred-2", PublicKey: []byte{9}}).Error; err != nil {
		t.Fatalf("seed other credential: %v", err)
	}
	rows, err := svc.List("a@example.com")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].CredentialID != "cred-1" {
		t.Fatalf("rows = %+v", rows)
	}
	if err := svc.Delete("a@example.com", rows[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows, _ = svc.List("a@example.com")
	if len(rows) != 0 {
		t.Fatalf("rows after delete = %+v", rows)
	}
	// The other account's credential must survive.
	var count int64
	svc.DB.Model(&models.WebauthnCredential{}).Count(&count)
	if count != 1 {
		t.Fatalf("credential count after delete = %d, want 1", count)
	}
}
