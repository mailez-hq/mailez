package core

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core/models"
)

func newTestApp(t *testing.T) (*fiber.App, *App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "core.db")), &gorm.Config{
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
	app := New(db, mgr, Config{SecretKey: "test-secret"})
	f := fiber.New()
	f.Get("/private", app.RequireAuth, func(c *fiber.Ctx) error { return c.SendStatus(200) })
	f.Get("/admin", app.RequireAuth, app.RequireGlobalAdmin, func(c *fiber.Ctx) error { return c.SendStatus(200) })
	f.Post("/write", app.Audit, func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", GlobalAdmin: true})
		return c.SendStatus(204)
	})
	return f, app
}

func TestRequireAuth(t *testing.T) {
	app, _ := newTestApp(t)
	resp, err := app.Test(httptest.NewRequest("GET", "/private", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuditRecordsWrites(t *testing.T) {
	app, a := newTestApp(t)
	resp, err := app.Test(httptest.NewRequest("POST", "/write", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	var count int64
	if err := a.DB.Model(&models.AuditLog{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit rows = %d, err=%v; want 1", count, err)
	}
}
