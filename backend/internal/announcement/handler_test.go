package announcement

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

func newTestApp(t *testing.T, globalAdmin bool) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "announcement.db")), &gorm.Config{
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
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", Enabled: true, GlobalAdmin: globalAdmin})
		return c.Next()
	})
	h.Register(authed)
	return f
}

func TestAnnouncementCRUD(t *testing.T) {
	app := newTestApp(t, true)

	// No announcement yet: GET is 204.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/announcement", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("get empty: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("get empty status = %d, want 204", resp.StatusCode)
	}

	// Publish a notice.
	body := `{"subject":"Maintenance","body":"Downtime tonight","enabled":true}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/announcement", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status = %d, want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var out struct {
		ID      uint   `json:"id"`
		Subject string `json:"subject"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.Subject != "Maintenance" || !out.Enabled {
		t.Fatalf("put body = %s", string(b))
	}

	// Users now see it.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/announcement", nil)
	resp, err = app.Test(req, -1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), "Downtime") {
		t.Fatalf("get = %s", string(b))
	}

	// Disabling hides it without deleting.
	body = `{"subject":"Maintenance","body":"Downtime tonight","enabled":false}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/announcement", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if resp, _ = app.Test(req, -1); resp.StatusCode != http.StatusOK {
		t.Fatalf("disable status = %d, want 200", resp.StatusCode)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/announcement", nil)
	if resp, _ = app.Test(req, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("disabled get status = %d, want 204", resp.StatusCode)
	}

	// Clearing removes it for good.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/announcement", nil)
	if resp, _ = app.Test(req, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/announcement", nil)
	if resp, _ = app.Test(req, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("get after delete status = %d, want 204", resp.StatusCode)
	}
}

func TestAnnouncementAdminOnly(t *testing.T) {
	app := newTestApp(t, false) // regular user
	req := httptest.NewRequest(http.MethodPut, "/api/v1/announcement", strings.NewReader(`{"subject":"x","body":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("put status = %d, want 403", resp.StatusCode)
	}
}
