package sieve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

type fakeGateway struct {
	mail.Gateway
	scripts []mail.SieveScript
	put     string
	active  string
	deleted string
}

func (f *fakeGateway) SieveListScripts(email, token string) ([]mail.SieveScript, error) {
	return f.scripts, nil
}

func (f *fakeGateway) SieveGetScript(email, token, name string) (string, error) {
	return "require [\"fileinto\"];", nil
}

func (f *fakeGateway) SievePutScript(email, token, name, content string, activate bool) error {
	f.put = name
	return nil
}

func (f *fakeGateway) SieveDeleteScript(email, token, name string) error {
	f.deleted = name
	return nil
}

func (f *fakeGateway) SieveSetActive(email, token, name string) error {
	f.active = name
	return nil
}

func newTestApp(t *testing.T, gw mail.Gateway) (*fiber.App, *fakeGateway) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sieve.db")), &gorm.Config{
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
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	fake, _ := gw.(*fakeGateway)
	app.Mail = fake
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	h.Register(authed)
	return f, fake
}

func TestSieveCRUD(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{scripts: []mail.SieveScript{{Name: "default", Active: true}}})
	put := httptest.NewRequest(http.MethodPut, "/api/v1/sieve/rules", strings.NewReader(`{"content":"require [\"fileinto\"];"}`))
	put.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(put); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put status = %d, want 204", resp.StatusCode)
	}
	if fake.put != "rules" {
		t.Fatalf("put name = %q", fake.put)
	}
	act := httptest.NewRequest(http.MethodPost, "/api/v1/sieve/rules/activate", nil)
	if resp, _ := app.Test(act); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("activate status = %d, want 204", resp.StatusCode)
	}
	if fake.active != "rules" {
		t.Fatalf("active = %q", fake.active)
	}
	del := httptest.NewRequest(http.MethodDelete, "/api/v1/sieve/rules", nil)
	if resp, _ := app.Test(del); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
	if fake.deleted != "rules" {
		t.Fatalf("deleted = %q", fake.deleted)
	}
}
