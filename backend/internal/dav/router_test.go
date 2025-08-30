package dav

import (
	"encoding/base64"
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

	"mailez/backend/internal/authcache"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

func newDAVApp(t *testing.T) (*gorm.DB, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "davroute.db")), &gorm.Config{
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
	if err := db.Create(&models.User{
		Email: "alice@example.com", Localpart: "alice", DomainName: "example.com",
		Password: mustHash(t, "secret123"), Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{
		RequestMethods: append(append([]string{}, fiber.DefaultMethods...), "PROPFIND", "REPORT"),
	})
	New(db, authcache.New(0)).Register(app.Group("/dav"))
	return db, app
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := password.Hash(pw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func basic(email, pw string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+pw))
}

func davReq(t *testing.T, app *fiber.App, method, path, body, auth string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func TestDAVAuthAndDiscovery(t *testing.T) {
	db, app := newDAVApp(t)

	// Missing credentials are rejected with 401 + WWW-Authenticate.
	resp := davReq(t, app, "PROPFIND", "/dav/", "", "")
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("no auth: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Wrong password is rejected.
	resp = davReq(t, app, "PROPFIND", "/dav/", "", basic("alice@example.com", "wrong"))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong pw: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// App token authenticates (as dedicated CalDAV/CardDAV clients do).
	appHash, err := password.HashPBKDF2SHA256("dav-app-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Token{UserEmail: "alice@example.com", Password: appHash}).Error; err != nil {
		t.Fatal(err)
	}
	resp = davReq(t, app, "PROPFIND", "/dav/", "", basic("alice@example.com", "dav-app-token"))
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("app token PROPFIND: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{"current-user-principal", "/dav/principals/alice@example.com/", "addressbook-home-set", "calendar-home-set"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("root discovery missing %q: %s", want, body)
		}
	}

	// Principal URL answers discovery for the authenticated user.
	resp = davReq(t, app, "PROPFIND", "/dav/principals/alice@example.com/", "", basic("alice@example.com", "dav-app-token"))
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("principal PROPFIND: %d", resp.StatusCode)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "addressbook-home-set") {
		t.Fatalf("principal discovery: %s", body)
	}

	// A principal URL naming another user is forbidden.
	resp = davReq(t, app, "PROPFIND", "/dav/principals/bob@example.com/", "", basic("alice@example.com", "dav-app-token"))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-user principal: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// PUT through the router stores the contact.
	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:r-1\r\nFN:Router Test\r\nEMAIL:r@test.com\r\nEND:VCARD\r\n"
	resp = davReq(t, app, http.MethodPut, "/dav/addressbooks/alice@example.com/default/r-1.vcf", vcard, basic("alice@example.com", "dav-app-token"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("router PUT: %d", resp.StatusCode)
	}
	resp.Body.Close()
	var count int64
	if err := db.Model(&models.Contact{}).Where("user_email = ? AND dav_uid = ?", "alice@example.com", "r-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("contact count: %d", count)
	}
}

// TestDAVCrossAccountIDOR locks the dispatch-level ownership check: every
// addressbook/calendar path segment must name the authenticated user, or a
// valid credential could read/overwrite/delete another account's data.
func TestDAVCrossAccountIDOR(t *testing.T) {
	db, app := newDAVApp(t)
	// A second victim user with existing data.
	if err := db.Create(&models.User{
		Email: "bob@example.com", Localpart: "bob", DomainName: "example.com",
		Password: mustHash(t, "bobsecret"), Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Contact{
		UserEmail: "bob@example.com", DavUID: "secret-1", Name: "Victim Card", Email: "victim@example.com",
	}).Error; err != nil {
		t.Fatal(err)
	}
	alice := basic("alice@example.com", "secret123")

	// Cross-account reads are forbidden at the router, never reaching the
	// backend scoped by the path user.
	for _, tc := range []struct{ method, path string }{
		{"PROPFIND", "/dav/addressbooks/bob@example.com/"},
		{"PROPFIND", "/dav/addressbooks/bob@example.com/default/"},
		{http.MethodGet, "/dav/addressbooks/bob@example.com/default/secret-1.vcf"},
		{http.MethodPut, "/dav/addressbooks/bob@example.com/default/evil.vcf"},
		{http.MethodDelete, "/dav/addressbooks/bob@example.com/default/secret-1.vcf"},
		{"REPORT", "/dav/addressbooks/bob@example.com/default/"},
		{"PROPFIND", "/dav/calendars/bob@example.com/"},
		{http.MethodPut, "/dav/calendars/bob@example.com/default/evil.ics"},
		{http.MethodDelete, "/dav/calendars/bob@example.com/default/x.ics"},
	} {
		resp := davReq(t, app, tc.method, tc.path, "", alice)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s %s: want 403, got %d", tc.method, tc.path, resp.StatusCode)
		}
	}

	// The victim's data is intact.
	var n int64
	if err := db.Model(&models.Contact{}).Where("user_email = ? AND dav_uid = ?", "bob@example.com", "secret-1").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("victim contact survived: %d", n)
	}

	// Own path still works.
	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a-1\r\nFN:Alice\r\nEND:VCARD\r\n"
	resp := davReq(t, app, http.MethodPut, "/dav/addressbooks/alice@example.com/default/a-1.vcf", vcard, alice)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("own PUT: %d", resp.StatusCode)
	}
}
