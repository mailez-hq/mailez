package admin

import (
	"encoding/json"
	"io"
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
)

func newTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "admin.db")), &gorm.Config{
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
	if err := db.Create(&models.Domain{Name: "t.example", MaxUsers: 5, MaxAliases: 5}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	h.Register(authed)
	return f
}

func TestConfigExportImportRoundtrip(t *testing.T) {
	app := newTestApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/config/export", nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var backup map[string]any
	if err := json.Unmarshal(b, &backup); err != nil {
		t.Fatalf("export json: %v", err)
	}
	domains, _ := backup["domains"].([]any)
	if len(domains) != 1 {
		t.Fatalf("exported domains = %d, want 1: %s", len(domains), string(b))
	}

	imp := httptest.NewRequest(http.MethodPost, "/api/v1/config/import", strings.NewReader(string(b)))
	imp.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(imp); resp.StatusCode != http.StatusOK {
		t.Fatalf("import status = %d, want 200", resp.StatusCode)
	}
}

func TestAuditList(t *testing.T) {
	app := newTestApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil))
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit status = %d, want 200", resp.StatusCode)
	}
}

func TestBrandingRoundtrip(t *testing.T) {
	app := newTestApp(t)

	// Fresh instance returns empty defaults.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil))
	if err != nil {
		t.Fatalf("branding get: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var empty map[string]any
	if err := json.Unmarshal(b, &empty); err != nil {
		t.Fatalf("branding json: %v", err)
	}
	if title, _ := empty["title"].(string); title != "" {
		t.Fatalf("initial title = %q, want empty", title)
	}

	// Save a custom brand.
	put := httptest.NewRequest(http.MethodPut, "/api/v1/branding", strings.NewReader(`{
		"title": "Example University",
		"subtitle": "电子邮件系统",
		"tagline": "安全、稳定、高效",
		"feature1": "特性一",
		"feature2": "特性二",
		"feature3": "特性三",
		"logo_url": "https://example.com/logo.png",
		"hero_url": "https://example.com/hero.jpg",
		"copyright": "Copyright © example.com, All Rights Reserved",
		"contact": "support@example.com"
	}`))
	put.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(put)
	if err != nil {
		t.Fatalf("branding put: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("branding put status = %d, body %s", resp.StatusCode, string(b))
	}

	// Read it back.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil))
	if err != nil {
		t.Fatalf("branding get after put: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("branding json after put: %v", err)
	}
	if got["title"] != "Example University" {
		t.Fatalf("title after put = %v, want Example University", got["title"])
	}
	if got["logo_url"] != "https://example.com/logo.png" {
		t.Fatalf("logo_url after put = %v", got["logo_url"])
	}
	if got["hero_url"] != "https://example.com/hero.jpg" {
		t.Fatalf("hero_url after put = %v", got["hero_url"])
	}
	if got["contact"] != "support@example.com" {
		t.Fatalf("contact after put = %v", got["contact"])
	}
}
