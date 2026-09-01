package invite

import (
	"encoding/base64"
	"encoding/json"
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
	"mailez/backend/internal/mail"
)

// fakeGateway records Send calls (the production gateway is too heavy for
// handler tests).
type fakeGateway struct {
	mail.Gateway
	sentTo     []string
	sentSubj   string
	sentAttach []mail.Attachment
}

func (f *fakeGateway) Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, extra ...mail.Header) error {
	f.sentTo = to
	f.sentSubj = subject
	f.sentAttach = attachments
	return nil
}

const requestICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Test//Test//EN\r\n" +
	"METHOD:REQUEST\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:meet-123@example.com\r\n" +
	"DTSTAMP:20260827T000000Z\r\n" +
	"DTSTART:20260901T020000Z\r\n" +
	"DTEND:20260901T030000Z\r\n" +
	"SUMMARY:季度评审\r\n" +
	"LOCATION:会议室A\r\n" +
	"ORGANIZER:mailto:boss@example.com\r\n" +
	"ATTENDEE:mailto:alice@example.com\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

const cancelICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Test//Test//EN\r\n" +
	"METHOD:CANCEL\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:meet-123@example.com\r\n" +
	"DTSTAMP:20260827T000000Z\r\n" +
	"ORGANIZER:mailto:boss@example.com\r\n" +
	"ATTENDEE:mailto:alice@example.com\r\n" +
	"STATUS:CANCELLED\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func newTestInvite(t *testing.T) (*Service, *fakeGateway, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invite.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	fake := &fakeGateway{}
	app := core.New(db, auth.NewManager(db, auth.NewMemoryStore(), "s", time.Hour), core.Config{})
	app.Mail = fake
	s := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "alice@example.com", Enabled: true})
		return c.Next()
	})
	s.Register(authed)
	return s, fake, f
}

func postJSON(t *testing.T, f *fiber.App, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRespondAccept(t *testing.T) {
	s, fake, f := newTestInvite(t)
	resp := postJSON(t, f, "/api/v1/invites/respond", map[string]string{
		"ics": requestICS, "action": "accept",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if len(fake.sentTo) != 1 || fake.sentTo[0] != "boss@example.com" {
		t.Fatalf("reply recipient = %v", fake.sentTo)
	}
	if !strings.HasPrefix(fake.sentSubj, "接受：") {
		t.Errorf("subject = %q", fake.sentSubj)
	}
	if len(fake.sentAttach) != 1 || fake.sentAttach[0].ContentType == "" {
		t.Fatalf("attachments = %+v", fake.sentAttach)
	}
	// The accepted meeting landed in the calendar.
	var ev models.CalendarEvent
	if err := s.DB.First(&ev, "user_email = ? AND uid = ?", "alice@example.com", "meet-123@example.com").Error; err != nil {
		t.Fatalf("calendar event: %v", err)
	}
	if ev.Summary != "季度评审" {
		t.Errorf("summary = %q", ev.Summary)
	}
}

func TestRespondDeclineRemovesEvent(t *testing.T) {
	s, fake, f := newTestInvite(t)
	if err := s.DB.Create(&models.CalendarEvent{
		UserEmail: "alice@example.com", UID: "meet-123@example.com", Summary: "季度评审", ICS: requestICS,
	}).Error; err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, f, "/api/v1/invites/respond", map[string]string{
		"ics": requestICS, "action": "decline",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.HasPrefix(fake.sentSubj, "拒绝：") {
		t.Errorf("subject = %q", fake.sentSubj)
	}
	var count int64
	s.DB.Model(&models.CalendarEvent{}).Where("user_email = ? AND uid = ?", "alice@example.com", "meet-123@example.com").Count(&count)
	if count != 0 {
		t.Errorf("declined event still present")
	}
}

func TestRespondCancelRemovesEvent(t *testing.T) {
	s, fake, f := newTestInvite(t)
	if err := s.DB.Create(&models.CalendarEvent{
		UserEmail: "alice@example.com", UID: "meet-123@example.com", Summary: "季度评审", ICS: requestICS,
	}).Error; err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, f, "/api/v1/invites/respond", map[string]string{
		"ics": cancelICS, "action": "cancel",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	// A CANCEL is not answered: no mail goes out, the event is gone.
	if fake.sentTo != nil {
		t.Errorf("cancel must not send a reply, sent to %v", fake.sentTo)
	}
	var count int64
	s.DB.Model(&models.CalendarEvent{}).Where("user_email = ? AND uid = ?", "alice@example.com", "meet-123@example.com").Count(&count)
	if count != 0 {
		t.Errorf("cancelled event still present")
	}
}

func TestRespondCancelByMethod(t *testing.T) {
	s, fake, f := newTestInvite(t)
	if err := s.DB.Create(&models.CalendarEvent{
		UserEmail: "alice@example.com", UID: "meet-123@example.com", Summary: "季度评审", ICS: requestICS,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// A real CANCEL notice arrives as METHOD:CANCEL; the reader sends the
	// raw ICS regardless of the guessed action.
	resp := postJSON(t, f, "/api/v1/invites/respond", map[string]string{
		"ics": cancelICS, "action": "accept",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if fake.sentTo != nil {
		t.Errorf("cancel by method must not send a reply, sent to %v", fake.sentTo)
	}
	var count int64
	s.DB.Model(&models.CalendarEvent{}).Where("user_email = ? AND uid = ?", "alice@example.com", "meet-123@example.com").Count(&count)
	if count != 0 {
		t.Errorf("cancelled event still present")
	}
}

func TestSendInvite(t *testing.T) {
	_, fake, f := newTestInvite(t)
	resp := postJSON(t, f, "/api/v1/invites/send", map[string]any{
		"to":      []string{"bob@example.com", "carol@example.com"},
		"summary": "产品评审", "location": "线上", "start": "2026-09-02T02:00:00Z",
		"end": "2026-09-02T03:00:00Z", "description": "评审新功能",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if len(fake.sentTo) != 2 {
		t.Fatalf("attendees = %v", fake.sentTo)
	}
	if len(fake.sentAttach) != 1 {
		t.Fatalf("attachments = %+v", fake.sentAttach)
	}
	// The attached ICS must be a valid REQUEST addressed to the attendees.
	icsText := fake.sentAttach[0].Data
	if icsText == "" {
		t.Fatal("missing ics data")
	}
	raw, err := decodeAttachment(icsText)
	if err != nil {
		t.Fatal(err)
	}
	inv := mail.ParseInvitation(raw)
	if inv == nil || inv.Method != "REQUEST" {
		t.Fatalf("parsed invite = %+v", inv)
	}
	if len(inv.Attendees) != 2 {
		t.Errorf("attendees in ics = %v", inv.Attendees)
	}
}

func decodeAttachment(b64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64)
}
