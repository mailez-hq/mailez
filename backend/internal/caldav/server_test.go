package caldav

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/webdav"
)

func testCalApp(t *testing.T) (*gorm.DB, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cal.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	cal := New(db)
	ws := &webdav.Server{Backend: cal}
	app := fiber.New(fiber.Config{
		RequestMethods: append(append([]string{}, fiber.DefaultMethods...), "PROPFIND", "REPORT"),
	})
	app.Use(func(c *fiber.Ctx) error {
		c.SetUserContext(webdav.WithUser(c.UserContext(), "alice@example.com"))
		return c.Next()
	})
	app.All("/dav/*", ws.Handle)
	return db, app
}

func TestCalDAVLifecycle(t *testing.T) {
	db, app := testCalApp(t)

	// PUT a calendar event (all-day + RRULE).
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Mailez//Test//EN\r\nBEGIN:VEVENT\r\nUID:evt-1\r\nDTSTAMP:20260827T000000Z\r\nDTSTART;VALUE=DATE:20260901\r\nDTEND;VALUE=DATE:20260902\r\nSUMMARY:Team Standup\r\nRRULE:FREQ=WEEKLY;BYDAY=TU,TH\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	req := httptest.NewRequest(http.MethodPut, "/dav/calendars/alice@example.com/default/evt-1.ics", strings.NewReader(ics))
	req.Header.Set("Content-Type", "text/calendar")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT event: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Structured columns were extracted (all-day + RRULE preserved).
	var ev models.CalendarEvent
	if err := db.Where("user_email = ? AND uid = ?", "alice@example.com", "evt-1").First(&ev).Error; err != nil {
		t.Fatal(err)
	}
	if !ev.AllDay || ev.RRule != "FREQ=WEEKLY;BYDAY=TU,TH" || ev.Summary != "Team Standup" {
		t.Fatalf("extracted event: %+v", ev)
	}

	// GET returns the raw ICS byte-for-byte.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/dav/calendars/alice@example.com/default/evt-1.ics", nil), 5000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET event: %d", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.TrimSpace(string(got)) != strings.TrimSpace(ics) {
		t.Fatalf("ICS round-trip mismatch:\n%s\n---\n%s", got, ics)
	}

	// calendar-query with a window returns the event.
	query := `<?xml version="1.0"?><C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><C:calendar-data/></D:prop><C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT"><C:time-range start="20260901T000000Z" end="20261231T000000Z"/></C:comp-filter></C:comp-filter></C:filter></C:calendar-query>`
	req = httptest.NewRequest("REPORT", "/dav/calendars/alice@example.com/default/", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/xml")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus || !strings.Contains(string(body), "Team Standup") {
		t.Fatalf("calendar-query: %d %s", resp.StatusCode, body)
	}

	// A window outside the event (before it starts, no overlap) excludes it
	// only when there is no RRULE; with RRULE the event is returned for any
	// future window (clients expand locally).
	queryPast := `<?xml version="1.0"?><C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><C:calendar-data/></D:prop><C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT"><C:time-range start="20240101T000000Z" end="20240131T000000Z"/></C:comp-filter></C:comp-filter></C:filter></C:calendar-query>`
	req = httptest.NewRequest("REPORT", "/dav/calendars/alice@example.com/default/", strings.NewReader(queryPast))
	req.Header.Set("Content-Type", "application/xml")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Team Standup") {
		t.Fatalf("RRULE event dropped for future query: %s", body)
	}

	// DELETE removes the event.
	resp, err = app.Test(httptest.NewRequest(http.MethodDelete, "/dav/calendars/alice@example.com/default/evt-1.ics", nil), 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE event: %d", resp.StatusCode)
	}
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/dav/calendars/alice@example.com/default/evt-1.ics", nil), 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after delete: %d", resp.StatusCode)
	}
}
