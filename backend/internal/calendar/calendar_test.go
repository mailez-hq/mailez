package calendar

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func newCalendarApp(t *testing.T) (*Handler, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "calapi.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := &models.User{Email: "alice@example.com", Enabled: true}
	h := New(core.New(db, nil, core.Config{}))
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", user)
		return c.Next()
	})
	h.Register(app.Group("/api/v1"))
	return h, app
}

func TestCalendarAPI(t *testing.T) {
	_, app := newCalendarApp(t)

	// Create an event.
	payload := `{"summary":"Sync Test","location":"Room 1","description":"desc","start":"2026-09-01T09:00:00+08:00","end":"2026-09-01T10:00:00+08:00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/calendar/events", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created fiber.Map
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created["summary"] != "Sync Test" || created["uid"] == "" {
		t.Fatalf("created: %+v", created)
	}
	id := uint(created["id"].(float64))

	// List within a window that contains the event.
	from := "2026-09-01T00:00:00+08:00"
	to := "2026-09-02T00:00:00+08:00"
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/calendar/events?from="+from+"&to="+to, nil), 5000)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Sync Test") {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}

	// Update.
	update := `{"summary":"Sync Test v2","start":"2026-09-02T09:00:00+08:00","end":"2026-09-02T10:00:00+08:00"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/calendar/events/"+strconv.FormatUint(uint64(id), 10), strings.NewReader(update))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Sync Test v2") {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/calendar/events/"+strconv.FormatUint(uint64(id), 10), nil)
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
}
