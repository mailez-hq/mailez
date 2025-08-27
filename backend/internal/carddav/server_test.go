package carddav

import (
	"bytes"
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

func testApp(t *testing.T) (*gorm.DB, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dav.db")), &gorm.Config{
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
	card := New(db)
	ws := &webdav.Server{Backend: card}
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

func doReq(t *testing.T, app *fiber.App, method, path, body, contentType string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func read(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCardDAVLifecycle(t *testing.T) {
	_, app := testApp(t)

	// Discovery on the addressbook home lists the collection.
	resp := doReq(t, app, "PROPFIND", "/dav/addressbooks/alice@example.com/", "", "")
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("home PROPFIND: %d body=%q", resp.StatusCode, read(t, resp))
	}
	home := read(t, resp)
	if !strings.Contains(home, "addressbook-home-set") || !strings.Contains(home, "/dav/addressbooks/alice@example.com/") {
		t.Fatalf("home discovery missing home set: %s", home)
	}

	// PUT a vCard.
	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:abc-123\r\nFN:Alice Test\r\nEMAIL:alice@test.com\r\nCATEGORIES:Work\r\nEND:VCARD\r\n"
	resp = doReq(t, app, http.MethodPut, "/dav/addressbooks/alice@example.com/default/abc-123.vcf", vcard, "text/vcard")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT contact: %d %s", resp.StatusCode, read(t, resp))
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("PUT returned no ETag")
	}
	resp.Body.Close()

	// GET returns the vCard with UID and REV.
	resp = doReq(t, app, http.MethodGet, "/dav/addressbooks/alice@example.com/default/abc-123.vcf", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET contact: %d", resp.StatusCode)
	}
	got := read(t, resp)
	for _, want := range []string{"UID:abc-123", "FN:Alice Test", "EMAIL", "alice@test.com", "REV:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("GET vCard missing %q: %s", want, got)
		}
	}

	// PROPFIND depth 1 on the collection lists the contact with its ETag.
	resp = doReq(t, app, "PROPFIND", "/dav/addressbooks/alice@example.com/default/", "",
		`<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:getetag/><D:resourcetype/></D:prop></D:propfind>`)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("collection PROPFIND: %d", resp.StatusCode)
	}
	col := read(t, resp)
	if !strings.Contains(col, "addressbook") || !strings.Contains(col, "abc-123.vcf") || !strings.Contains(col, "getetag") {
		t.Fatalf("collection listing missing entries: %s", col)
	}

	// REPORT addressbook-query returns the address-data.
	report := `<?xml version="1.0"?><C:addressbook-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav"><D:prop><C:address-data/></D:prop></C:addressbook-query>`
	resp = doReq(t, app, "REPORT", "/dav/addressbooks/alice@example.com/default/", report, "application/xml")
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("REPORT: %d", resp.StatusCode)
	}
	query := read(t, resp)
	if !strings.Contains(query, "BEGIN:VCARD") || !strings.Contains(query, "Alice Test") {
		t.Fatalf("addressbook-query missing card data: %s", query)
	}

	// Conditional PUT: wrong If-Match is rejected with 412.
	resp = doReq(t, app, http.MethodPut, "/dav/addressbooks/alice@example.com/default/abc-123.vcf", vcard, "text/vcard")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second PUT: %d", resp.StatusCode)
	}
	req := httptest.NewRequest(http.MethodPut, "/dav/addressbooks/alice@example.com/default/abc-123.vcf", bytes.NewBufferString(vcard))
	req.Header.Set("If-Match", `"bogus"`)
	req.Header.Set("Content-Type", "text/vcard")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("If-Match mismatch: %d", resp.StatusCode)
	}

	// DELETE removes the contact.
	resp = doReq(t, app, http.MethodDelete, "/dav/addressbooks/alice@example.com/default/abc-123.vcf", "", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doReq(t, app, http.MethodGet, "/dav/addressbooks/alice@example.com/default/abc-123.vcf", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
}
