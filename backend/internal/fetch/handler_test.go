package fetch

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

func newFetchTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fetches.db")), &gorm.Config{
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
	if err := db.Create(&models.User{Email: "a@example.com", Localpart: "a", DomainName: "example.com", Enabled: true}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	RegisterAPI(authed, app)
	return f
}

func TestFetchCRUD(t *testing.T) {
	app := newFetchTestApp(t)
	body := `{"user_email":"a@example.com","protocol":"pop3","host":"mail.example.net","port":110,"username":"u","password":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fetches", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var f models.Fetch
	if err := json.Unmarshal(b, &f); err != nil || f.Host != "mail.example.net" {
		t.Fatalf("created fetch = %s", string(b))
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/fetches?user=a@example.com", nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lb, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	var page struct {
		Data  []models.Fetch `json:"data"`
		Total int            `json:"total"`
	}
	if err := json.Unmarshal(lb, &page); err != nil || page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("list = %s", string(lb))
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/fetches/1", nil)
	if resp, _ := app.Test(del, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
}
