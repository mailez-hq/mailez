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
	resp, err := app.Test(req, -1)
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

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/users?domain=t.example", nil), -1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lb, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	var page struct {
		Data  []models.User `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(lb, &page); err != nil || page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("list = %s", string(lb))
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/users/amy@t.example", nil)
	if resp, _ := app.Test(del, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
}

func TestUserCreateDuplicateReturnsConflict(t *testing.T) {
	app := newUserTestApp(t)
	body := `{"email":"amy@t.example","password":"secret-pass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(req, -1); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create: %v status %d", err, resp.StatusCode)
	}
	dup := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	dup.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(dup, -1)
	if err != nil {
		t.Fatalf("dup create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("dup status = %d, want 409", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "already exists") {
		t.Fatalf("dup body = %s, want clear message", string(b))
	}
}

func TestUserListSearch(t *testing.T) {
	app := newUserTestApp(t)
	for _, email := range []string{"alice@t.example", "bob@t.example", "carol@t.example"} {
		body := `{"email":"` + email + `","password":"secret-pass","displayed_name":"` + strings.ToUpper(email[:1]) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if resp, err := app.Test(req, -1); err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: %v status %d", email, err, resp.StatusCode)
		}
	}
	type page struct {
		Data  []models.User `json:"data"`
		Total int           `json:"total"`
	}
	// Match by email fragment.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/users?q=li", nil), -1)
	if err != nil {
		t.Fatalf("search email: %v", err)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if p.Total != 1 || len(p.Data) != 1 || p.Data[0].Email != "alice@t.example" {
		t.Fatalf("search 'li' = %+v", p)
	}
	// Match by displayed name (case-insensitive on SQLite LIKE).
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/users?q=BOB", nil), -1)
	if err != nil {
		t.Fatalf("search name: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if p.Total != 1 || len(p.Data) != 1 || p.Data[0].Email != "bob@t.example" {
		t.Fatalf("search 'BOB' = %+v", p)
	}
}
