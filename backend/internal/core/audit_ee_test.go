//go:build mailez_ee

package core

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

func TestClassifyAudit(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		query  string
		action string
		target string
		detail string
	}{
		{"collection create", "POST", "/api/v1/users", "", "users.create", "", ""},
		{"item update", "PUT", "/api/v1/users/bob@example.com", "", "users.update", "bob@example.com", ""},
		{"item delete", "DELETE", "/api/v1/domains/d.io", "", "domains.delete", "d.io", ""},
		{"row id delete", "DELETE", "/api/v1/tokens/42", "", "tokens.delete", "42", ""},
		{"sub-resource action", "POST", "/api/v1/domains/d.io/dkim", "", "domains.dkim", "d.io", ""},
		{"self-service", "PUT", "/api/v1/me/password", "", "me.update", "password", ""},
		{"escaped email", "DELETE", "/api/v1/users/bob%40example.com", "", "users.delete", "bob@example.com", ""},
		{"query kept as detail", "POST", "/api/v1/aliases", "domain=d.io", "aliases.create", "", "domain=d.io"},
		{"non-api path", "POST", "/health", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, target, detail := classifyAudit(tc.method, tc.path, tc.query)
			if action != tc.action || target != tc.target || detail != tc.detail {
				t.Fatalf("classifyAudit(%s %s %s) = (%q %q %q), want (%q %q %q)",
					tc.method, tc.path, tc.query, action, target, detail, tc.action, tc.target, tc.detail)
			}
		})
	}
}

func TestAuditMiddlewareEnriches(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audit.db")), &gorm.Config{
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
	app := New(db, nil, Config{SecretKey: "test-secret"})
	f := fiber.New()
	f.Put("/api/v1/users/bob%40example.com", app.Audit, func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "admin@example.com"})
		return c.SendStatus(204)
	})
	resp, err := f.Test(httptest.NewRequest("PUT", "/api/v1/users/bob%40example.com", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	var entry models.AuditLog
	if err := db.First(&entry).Error; err != nil {
		t.Fatalf("load entry: %v", err)
	}
	if entry.Action != "users.update" || entry.Target != "bob@example.com" {
		t.Fatalf("enriched = (%q %q), want (users.update bob@example.com)", entry.Action, entry.Target)
	}
}
