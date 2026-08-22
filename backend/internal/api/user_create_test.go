package api

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

	"mailess/backend/internal/auth"
	"mailess/backend/internal/config"
	"mailess/backend/internal/models"
)

func newUserCreateHarness(t *testing.T) (*Handler, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{
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
	cfg := config.Config{SecretKey: "test-secret"}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailess_session", time.Hour)
	h := New(db, mgr, cfg)
	app := fiber.New()
	// Act as a global admin; role enforcement is covered elsewhere.
	mw := func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{GlobalAdmin: true})
		return c.Next()
	}
	h.registerUsers(app.Group("/api/v1"), mw)
	return h, app
}

func TestCreateUserDisabledPersists(t *testing.T) {
	h, app := newUserCreateHarness(t)
	if err := h.DB.Create(&models.Domain{Name: "example.com"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	create := func(enabled string) models.User {
		t.Helper()
		body := `{"email":"u` + enabled + `@example.com","password":"secret123","enabled":` + enabled + `}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 201 {
			t.Fatalf("create user status: %d %s", resp.StatusCode, b)
		}
		var created models.User
		if err := json.Unmarshal(b, &created); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return created
	}

	// Explicitly disabled accounts must stay disabled in storage.
	created := create("false")
	if created.Enabled {
		t.Fatal("response reports enabled for a disabled account")
	}
	var stored models.User
	if err := h.DB.First(&stored, "email = ?", created.Email).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Enabled {
		t.Fatal("disabled account was stored as enabled (GORM default-tag omission)")
	}

	// Explicitly enabled and omitted both default to enabled.
	enabled := create("true")
	if !enabled.Enabled {
		t.Fatal("explicitly enabled account stored disabled")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users",
		strings.NewReader(`{"email":"default@example.com","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("create default user: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var def models.User
	if resp.StatusCode != 201 || json.Unmarshal(b, &def) != nil || !def.Enabled {
		t.Fatalf("default user: %d %s", resp.StatusCode, b)
	}
}
