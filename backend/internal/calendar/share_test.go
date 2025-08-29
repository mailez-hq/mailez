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
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

type calFake struct {
	mail.Gateway
}

func (f *calFake) With(dial mail.Dial) mail.Gateway { return f }

func newCalApp(t *testing.T, seed func(db *gorm.DB)) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cal.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.User{}, &models.CalendarEvent{}, &models.CalendarShare{}, &models.CalendarReminderLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Create(&models.User{Email: "alice@example.com", Localpart: "alice", DomainName: "example.com", Enabled: true})
	db.Create(&models.User{Email: "bob@example.com", Localpart: "bob", DomainName: "example.com", Enabled: true})
	if seed != nil {
		seed(db)
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	app.Mail = &calFake{}
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "bob@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	h.Register(authed)
	// Mirror the server wiring: the ICS export must be reachable WITHOUT the
	// session middleware — external subscribers carry only the feed token.
	// If export.ics ever moves back into Register, these requests 404.
	h.RegisterPublic(f.Group("/api/v1"))
	return f
}

func doCal(t *testing.T, app *fiber.App, method, path, body string) (*http.Response, string) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestCalendarSharesAndSharedEvents(t *testing.T) {
	// Simpler flow: alice shares to bob in the seed, bob lists events.
	app := newCalApp(t, func(db *gorm.DB) {
		start := time.Now().Add(time.Hour)
		db.Create(&models.CalendarEvent{
			UserEmail: "alice@example.com", UID: "ev-1", Summary: "评审会",
			Start: &start, ICS: "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
		})
		db.Create(&models.CalendarShare{OwnerEmail: "alice@example.com", ShareeEmail: "bob@example.com", ReadOnly: true})
		db.Create(&models.CalendarEvent{
			UserEmail: "bob@example.com", UID: "ev-2", Summary: "Bob 自己的日程",
			Start: &start, ICS: "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
		})
	})
	resp, body := doCal(t, app, http.MethodGet, "/api/v1/calendar/events", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(body), &events); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (own + shared)", len(events))
	}
	shared := false
	for _, e := range events {
		if e["summary"] == "评审会" {
			if e["owner_email"] != "alice@example.com" || e["read_only"] != true {
				t.Fatalf("shared event flags: %v", e)
			}
			shared = true
		}
	}
	if !shared {
		t.Fatal("shared event missing")
	}
}

func TestCalendarShareCreateAndRevoke(t *testing.T) {
	app := newCalApp(t, nil)
	resp, body := doCal(t, app, http.MethodPost, "/api/v1/calendar/shares",
		`{"sharee_email":"alice@example.com","read_only":false}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share status = %d: %s", resp.StatusCode, body)
	}
	var share map[string]any
	if err := json.Unmarshal([]byte(body), &share); err != nil {
		t.Fatalf("share result: %v", err)
	}
	if share["owner_email"] != "bob@example.com" || share["read_only"] != false {
		t.Fatalf("share = %v", share)
	}
	id := int(share["id"].(float64))
	resp2, _ := doCal(t, app, http.MethodDelete, "/api/v1/calendar/shares/"+strconv.Itoa(id), "")
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d", resp2.StatusCode)
	}
}

func TestCalendarExportToken(t *testing.T) {
	app := newCalApp(t, func(db *gorm.DB) {
		start := time.Now().Add(time.Hour)
		db.Create(&models.CalendarEvent{
			UserEmail: "alice@example.com", UID: "ev-1", Summary: "导出测试",
			Start: &start, ICS: "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
		})
	})
	// Bad token rejected.
	resp, _ := doCal(t, app, http.MethodGet, "/api/v1/calendar/export.ics?email=alice@example.com&token=bad", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token status = %d", resp.StatusCode)
	}
	// Good token streams the calendar.
	token := feedToken("alice@example.com", "test-secret")
	resp2, body2 := doCal(t, app, http.MethodGet,
		"/api/v1/calendar/export.ics?email=alice@example.com&token="+token, "")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d", resp2.StatusCode)
	}
	if !strings.Contains(body2, "导出测试") {
		t.Fatalf("export body missing event: %s", body2)
	}
}
