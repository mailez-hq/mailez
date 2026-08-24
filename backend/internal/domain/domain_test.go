package domain

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

func newDomainTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "domains.db")), &gorm.Config{
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
		c.Locals("user", &models.User{Email: "a@example.com", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	h.Register(authed)
	return f
}

func TestDomainCRUD(t *testing.T) {
	app := newDomainTestApp(t)
	body := `{"name":"new.example","max_users":5,"max_aliases":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	b, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	var page struct {
		Data  []models.Domain `json:"data"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(b, &page); err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].Name != "new.example" {
		t.Fatalf("list = %s", string(b))
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/new.example", nil)
	if resp, _ := app.Test(del, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
}
