package announcement

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// TestRegisterDoesNotGateSiblingRoutes is a regression test for a fiber
// pitfall: registering Group("", RequireGlobalAdmin) on the shared router
// installs a global Use middleware that 403s every later sibling route
// (calendar, drive, archive, invites, DLP, uploads) for normal users.
func TestRegisterDoesNotGateSiblingRoutes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "announcement.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}

	app := fiber.New()
	api := app.Group("/api/v1")
	api.Use(func(c *fiber.Ctx) error {
		// Simulate RequireAuth for a normal (non-admin) user.
		c.Locals("user", &models.User{Email: "u@example.com", Enabled: true})
		return c.Next()
	})
	New(&core.App{DB: db}).Register(api)
	// A sibling route registered after announcement must stay user-accessible.
	api.Get("/calendar/events", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/events", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sibling route gated by announcement admin middleware: got %d, want 200", resp.StatusCode)
	}

	// The announcement write routes themselves must still require admin.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/announcement", nil)
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatalf("announcement put: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("announcement write must require admin: got %d, want 403", resp.StatusCode)
	}
}
