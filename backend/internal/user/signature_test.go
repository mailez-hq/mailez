package user

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
)

func newSignatureApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sigs.db")), &gorm.Config{
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
	if err := db.Create(&models.User{
		Email: "a@example.com", Localpart: "a", DomainName: "example.com",
		Enabled: true, DisplayedName: "Ada",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true, DisplayedName: "Ada"})
		return c.Next()
	})
	h.registerMe(authed)
	h.registerSignatures(authed)
	return f, db
}

func callAPI(t *testing.T, app *fiber.App, method, path, body string) (int, []byte) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func decodeSignature(t *testing.T, raw []byte) models.Signature {
	t.Helper()
	var sig models.Signature
	if err := json.Unmarshal(raw, &sig); err != nil {
		t.Fatalf("decode signature %s: %v", raw, err)
	}
	return sig
}

func TestSignatureLifecycle(t *testing.T) {
	app, _ := newSignatureApp(t)

	status, raw := callAPI(t, app, http.MethodPost, "/api/v1/me/signatures",
		`{"name":"Work","body_html":"<p>Ada <strong>Lovelace</strong></p>"}`)
	if status != http.StatusOK {
		t.Fatalf("create: status %d body %s", status, raw)
	}
	first := decodeSignature(t, raw)
	if !first.DefaultForNew || !first.DefaultForReply {
		t.Fatalf("first signature should be the default: %+v", first)
	}
	if first.BodyText != "Ada Lovelace" {
		t.Fatalf("body text projection wrong: %q", first.BodyText)
	}

	status, raw = callAPI(t, app, http.MethodPost, "/api/v1/me/signatures",
		`{"name":"Personal","body_html":"<p>Ada</p>"}`)
	if status != http.StatusOK {
		t.Fatalf("create second: status %d body %s", status, raw)
	}
	second := decodeSignature(t, raw)
	if second.DefaultForNew {
		t.Fatalf("second signature must not be default: %+v", second)
	}

	status, raw = callAPI(t, app, http.MethodPut, "/api/v1/me/signatures/"+itoa(second.ID)+"/default",
		`{"kind":"new","enabled":true}`)
	if status != http.StatusOK {
		t.Fatalf("set default: status %d body %s", status, raw)
	}
	if promoted := decodeSignature(t, raw); !promoted.DefaultForNew {
		t.Fatalf("signature not promoted: %+v", promoted)
	}
	var rows []models.Signature
	status, raw = callAPI(t, app, http.MethodGet, "/api/v1/me/signatures", "")
	if status != http.StatusOK {
		t.Fatalf("list: status %d body %s", status, raw)
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 signatures, got %d", len(rows))
	}
	defaults := 0
	for _, r := range rows {
		if r.DefaultForNew {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly one new-message default, got %d", defaults)
	}

	status, raw = callAPI(t, app, http.MethodPut, "/api/v1/me/signatures/"+itoa(first.ID),
		`{"name":"Work v2","identity_email":"Sales@Example.com","body_html":"<p>Ada L.</p>","default_for_new":true,"default_for_reply":false}`)
	if status != http.StatusOK {
		t.Fatalf("update: status %d body %s", status, raw)
	}
	updated := decodeSignature(t, raw)
	if updated.Name != "Work v2" || updated.IdentityEmail != "sales@example.com" {
		t.Fatalf("update not applied: %+v", updated)
	}
	if !updated.DefaultForNew || updated.DefaultForReply {
		t.Fatalf("update defaults wrong: %+v", updated)
	}
	var genericDefaultID uint
	for _, r := range rows {
		if r.IdentityEmail == "" && r.DefaultForNew {
			genericDefaultID = r.ID
		}
	}
	if genericDefaultID != second.ID {
		t.Fatalf("generic default changed unexpectedly: %+v", rows)
	}

	status, raw = callAPI(t, app, http.MethodDelete, "/api/v1/me/signatures/"+itoa(second.ID), "")
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d body %s", status, raw)
	}
	status, raw = callAPI(t, app, http.MethodGet, "/api/v1/me/signatures", "")
	if status != http.StatusOK {
		t.Fatalf("list after delete: %d", status)
	}
	rows = nil
	json.Unmarshal(raw, &rows)
	if len(rows) != 1 || rows[0].IdentityEmail != "sales@example.com" {
		t.Fatalf("unexpected rows after delete: %+v", rows)
	}
}

func TestSignatureDeletePromotesSibling(t *testing.T) {
	app, _ := newSignatureApp(t)
	var ids []uint
	for _, name := range []string{"A", "B"} {
		status, raw := callAPI(t, app, http.MethodPost, "/api/v1/me/signatures",
			`{"name":"`+name+`","body_html":"<p>`+name+`</p>"}`)
		if status != http.StatusOK {
			t.Fatalf("create %s: status %d body %s", name, status, raw)
		}
		ids = append(ids, decodeSignature(t, raw).ID)
	}

	status, raw := callAPI(t, app, http.MethodDelete, "/api/v1/me/signatures/"+itoa(ids[0]), "")
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d body %s", status, raw)
	}
	status, raw = callAPI(t, app, http.MethodGet, "/api/v1/me/signatures", "")
	if status != http.StatusOK {
		t.Fatalf("list after delete: %d", status)
	}
	var rows []models.Signature
	json.Unmarshal(raw, &rows)
	if len(rows) != 1 || rows[0].ID != ids[1] {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if !rows[0].DefaultForNew || !rows[0].DefaultForReply {
		t.Fatalf("surviving signature should inherit the defaults: %+v", rows[0])
	}
}

func TestSignatureValidation(t *testing.T) {
	app, _ := newSignatureApp(t)
	cases := []struct {
		name string
		body string
	}{
		{"empty body", `{"name":"X","body_html":"<p><br></p>"}`},
		{"too large", `{"name":"X","body_html":"<p>` + strings.Repeat("x", maxSignatureHTMLBytes+1) + `</p>"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := callAPI(t, app, http.MethodPost, "/api/v1/me/signatures", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (%s)", status, raw)
			}
		})
	}
}

func TestSignatureSanitisesHTML(t *testing.T) {
	app, _ := newSignatureApp(t)
	status, raw := callAPI(t, app, http.MethodPost, "/api/v1/me/signatures",
		`{"name":"Styled","body_html":"<p style=\"color: #ff0000\" onclick=\"steal()\">Hi</p><script>alert(1)</script><img src=\"https://x.example/a.png\">"}`)
	if status != http.StatusOK {
		t.Fatalf("create: status %d body %s", status, raw)
	}
	sig := decodeSignature(t, raw)
	if strings.Contains(strings.ToLower(sig.BodyHTML), "script") || strings.Contains(strings.ToLower(sig.BodyHTML), "onclick") {
		t.Fatalf("executable markup survived: %s", sig.BodyHTML)
	}
	if !strings.Contains(sig.BodyHTML, "color") {
		t.Fatalf("inline styling was dropped: %s", sig.BodyHTML)
	}
	if !strings.Contains(sig.BodyHTML, "https://x.example/a.png") {
		t.Fatalf("image lost: %s", sig.BodyHTML)
	}
}

func TestSignatureOwnershipIsolated(t *testing.T) {
	app, db := newSignatureApp(t)
	other := models.Signature{UserEmail: "b@example.com", Name: "B", BodyText: "b", BodyHTML: "<p>b</p>"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("seed other: %v", err)
	}
	status, raw := callAPI(t, app, http.MethodDelete, "/api/v1/me/signatures/"+itoa(other.ID), "")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for another user's signature, got %d (%s)", status, raw)
	}
	var count int64
	db.Model(&models.Signature{}).Where("id = ?", other.ID).Count(&count)
	if count != 1 {
		t.Fatalf("another user's signature was deleted")
	}
}

func TestLegacySettingsSignatureStillWorks(t *testing.T) {
	app, _ := newSignatureApp(t)
	status, raw := callAPI(t, app, http.MethodPut, "/api/v1/me/settings", `{"signature":"Ada\nada@example.com"}`)
	if status != http.StatusOK {
		t.Fatalf("legacy save: status %d body %s", status, raw)
	}
	status, raw = callAPI(t, app, http.MethodGet, "/api/v1/me", "")
	if status != http.StatusOK {
		t.Fatalf("profile: status %d", status)
	}
	var profile struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profile.Signature != "Ada\nada@example.com" {
		t.Fatalf("legacy field not mirrored: %q", profile.Signature)
	}
	status, raw = callAPI(t, app, http.MethodGet, "/api/v1/me/signatures", "")
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	var rows []models.Signature
	json.Unmarshal(raw, &rows)
	if len(rows) != 1 || !rows[0].DefaultForNew || rows[0].BodyText != "Ada\nada@example.com" {
		t.Fatalf("legacy save did not create the default signature: %+v", rows)
	}
	status, raw = callAPI(t, app, http.MethodPut, "/api/v1/me/settings", `{"signature":""}`)
	if status != http.StatusOK {
		t.Fatalf("legacy clear: status %d body %s", status, raw)
	}
	status, raw = callAPI(t, app, http.MethodGet, "/api/v1/me/signatures", "")
	rows = nil
	json.Unmarshal(raw, &rows)
	if len(rows) != 1 || rows[0].DefaultForNew || rows[0].DefaultForReply {
		t.Fatalf("legacy clear should only drop defaults: %+v", rows)
	}
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
