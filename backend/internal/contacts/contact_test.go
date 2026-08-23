package contacts

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

func newTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "contacts.db")), &gorm.Config{
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
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	h.Register(authed)
	return f
}

func TestContactCRUD(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", strings.NewReader(`{"name":"Amy","email":"amy@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/contacts", nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	b, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	var list []models.Contact
	if err := json.Unmarshal(b, &list); err != nil || len(list) != 1 || list[0].Email != "amy@example.com" {
		t.Fatalf("list = %s, want one amy contact", string(b))
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/contacts/1", nil)
	delResp, err := app.Test(del)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", delResp.StatusCode)
	}
}

func TestVCardRoundTrip(t *testing.T) {
	src := "BEGIN:VCARD\r\n" +
		"VERSION:3.0\r\n" +
		"FN:Bob Smith\r\n" +
		"EMAIL;TYPE=INTERNET:bob@example.com\r\n" +
		"NOTE:Work colleague\r\n" +
		"CATEGORIES:work,friends\r\n" +
		"PHOTO;VALUE=URI:https://example.com/bob.png\r\n" +
		"END:VCARD\r\n" +
		"BEGIN:VCARD\r\n" +
		"VERSION:3.0\r\n" +
		"FN:Carol\r\n" +
		"EMAIL;TYPE=INTERNET:carol@example.com\r\n" +
		"END:VCARD\r\n"
	parsed := ParseVCard(src)
	if len(parsed) != 2 {
		t.Fatalf("ParseVCard = %d entries, want 2", len(parsed))
	}
	if parsed[0].Name != "Bob Smith" || parsed[0].Email != "bob@example.com" ||
		parsed[0].Comment != "Work colleague" || parsed[0].Groups != "work,friends" ||
		parsed[0].Avatar != "https://example.com/bob.png" {
		t.Fatalf("ParseVCard[0] = %+v", parsed[0])
	}

	// Encode/parse round-trip preserves fields.
	encoded := EncodeVCard([]models.Contact{{
		Name: "Bob Smith", Email: "bob@example.com",
		Comment: "Work colleague", Groups: "work,friends", Avatar: "https://example.com/bob.png",
	}})
	back := ParseVCard(encoded)
	if len(back) != 1 || back[0].Name != "Bob Smith" || back[0].Groups != "work,friends" {
		t.Fatalf("round-trip = %+v", back)
	}
}

func TestContactImportExport(t *testing.T) {
	app := newTestApp(t)

	// Seed one contact, then export.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", strings.NewReader(`{"name":"Amy","email":"amy@example.com","groups":"work"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d err=%v", resp.StatusCode, err)
	}
	resp.Body.Close()

	expResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/contacts/export", nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if ct := expResp.Header.Get("Content-Type"); ct != "text/vcard; charset=utf-8" {
		t.Fatalf("export content-type = %q", ct)
	}
	b, _ := io.ReadAll(expResp.Body)
	expResp.Body.Close()
	body := string(b)
	if !strings.Contains(body, "FN:Amy") || !strings.Contains(body, "amy@example.com") || !strings.Contains(body, "CATEGORIES:work") {
		t.Fatalf("export body missing fields:\n%s", body)
	}

	// Import two new cards (one duplicate -> only one added).
	importBody := `{"data":"BEGIN:VCARD\nVERSION:3.0\nFN:Bob\nEMAIL;TYPE=INTERNET:bob@example.com\nEND:VCARD\nBEGIN:VCARD\nVERSION:3.0\nFN:Carol\nEMAIL;TYPE=INTERNET:carol@example.com\nEND:VCARD\nBEGIN:VCARD\nVERSION:3.0\nFN:Amy\nEMAIL;TYPE=INTERNET:amy@example.com\nEND:VCARD\n"}`
	impReq := httptest.NewRequest(http.MethodPost, "/api/v1/contacts/import", strings.NewReader(importBody))
	impReq.Header.Set("Content-Type", "application/json")
	impResp, err := app.Test(impReq)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	ib, _ := io.ReadAll(impResp.Body)
	impResp.Body.Close()
	var result struct {
		Added int `json:"added"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(ib, &result); err != nil {
		t.Fatalf("import decode: %v (%s)", err, string(ib))
	}
	if result.Added != 2 || result.Total != 3 {
		t.Fatalf("import result = %+v, want added=2 total=3", result)
	}
}
