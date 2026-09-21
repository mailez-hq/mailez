package user

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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

// TestUpdateUserDisconnectsEngineSessions covers the revocation path:
// disabling the account, turning IMAP off and rotating the password must
// reach the engine, not just the next login.
func TestUpdateUserDisconnectsEngineSessions(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []string
		authz []string
	)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		authz = append(authz, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer engine.Close()

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
	app := core.New(db, mgr, core.Config{
		SecretKey:            "test-secret",
		MailEngineMgmtAddr:   engine.URL,
		MailEngineMgmtSecret: "engine-secret",
	})
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "admin@example.com", DomainName: "example.com", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	h.registerUsers(authed, h.RequireManager)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"email":"amy@t.example","password":"secret-pass"}`))
	create.Header.Set("Content-Type", "application/json")
	resp, err := f.Test(create, -1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}

	put := func(body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/users/amy%40t.example", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := f.Test(req, -1)
		if err != nil {
			t.Fatalf("update %s: %v", body, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("update %s: status %d", body, resp.StatusCode)
		}
	}

	put(`{"enable_imap": false}`)
	mu.Lock()
	first := append([]string(nil), calls...)
	mu.Unlock()
	if len(first) != 1 || first[0] != "POST /v1/accounts/amy@t.example/disconnect" {
		t.Fatalf("enable_imap off: engine calls = %v", first)
	}
	if authz[0] != "Bearer engine-secret" {
		t.Fatalf("engine call authorization = %q", authz[0])
	}

	// A quota-only change leaves sessions up.
	put(`{"quota_bytes": 2048}`)
	if len(calls) != 1 {
		t.Fatalf("quota-only update should not disconnect, calls = %v", calls)
	}

	put(`{"enabled": false}`)
	put(`{"password": "another-secret"}`)
	if len(calls) != 3 {
		t.Fatalf("disable and password rotation should each disconnect, calls = %v", calls)
	}
}
