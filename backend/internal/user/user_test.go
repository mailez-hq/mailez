package user

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

func newUserTestApp(t *testing.T) *fiber.App {
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
	if err := db.Create(&models.Domain{Name: "t.example", MaxUsers: 10, MaxAliases: 10}).Error; err != nil {
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
	h.registerUsers(authed, h.RequireManager)
	return f
}

func TestUserCRUD(t *testing.T) {
	app := newUserTestApp(t)
	body := `{"email":"amy@t.example","password":"secret-pass","quota_bytes":1000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var u models.User
	if err := json.Unmarshal(b, &u); err != nil || u.Email != "amy@t.example" {
		t.Fatalf("created user = %s", string(b))
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/users?domain=t.example", nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lb, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	var users []models.User
	if err := json.Unmarshal(lb, &users); err != nil || len(users) != 1 {
		t.Fatalf("list = %s", string(lb))
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/users/amy@t.example", nil)
	if resp, _ := app.Test(del); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
}
