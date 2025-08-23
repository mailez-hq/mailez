package push

import (
	"encoding/json"
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
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "push.db")), &gorm.Config{
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
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	RegisterAPI(authed, app)
	return f
}

func TestPushVapidAndSubscribe(t *testing.T) {
	app := newTestApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/push/vapid", nil))
	if err != nil {
		t.Fatalf("vapid: %v", err)
	}
	var v struct {
		PublicKey string `json:"public_key"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&v)
	resp.Body.Close()
	if v.PublicKey == "" {
		t.Fatal("vapid public key must be generated")
	}
	body := `{"endpoint":"https://push.example.test/a","keys":{"p256dh":"AQI","auth":"AQI"}}`
	sub := httptest.NewRequest(http.MethodPost, "/api/v1/push/subscribe", strings.NewReader(body))
	sub.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(sub); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("subscribe status = %d, want 204", resp.StatusCode)
	}
	unsub := httptest.NewRequest(http.MethodDelete, "/api/v1/push/subscribe", strings.NewReader(`{"endpoint":"https://push.example.test/a"}`))
	unsub.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(unsub); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unsubscribe status = %d, want 204", resp.StatusCode)
	}
}
