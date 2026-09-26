//go:build mailez_ee

package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

func footerRequest(t *testing.T, app *fiber.App, method, path, body string) (int, []byte) {
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

func TestOrgFooterSaveListAndSanitise(t *testing.T) {
	app, _ := newTestApp(t)
	body := `{"domain":"T.Example","enabled":true,"body_html":"<p style=\"color: #ff0000\" onclick=\"steal()\">ACME Confidential</p><script>alert(1)</script>"}`
	status, raw := footerRequest(t, app, http.MethodPut, "/api/v1/admin/org-footer", body)
	if status != http.StatusOK {
		t.Fatalf("save status %d: %s", status, raw)
	}
	var saved models.OrgFooter
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.Domain != "t.example" || !saved.Enabled {
		t.Fatalf("unexpected row: %+v", saved)
	}
	low := strings.ToLower(saved.BodyHTML)
	if strings.Contains(low, "script") || strings.Contains(low, "onclick") {
		t.Fatalf("executable markup survived: %s", saved.BodyHTML)
	}
	if !strings.Contains(saved.BodyHTML, "color") {
		t.Fatalf("styling was dropped: %s", saved.BodyHTML)
	}
	if saved.BodyText != "ACME Confidential" {
		t.Fatalf("body_text projection wrong: %q", saved.BodyText)
	}

	status, raw = footerRequest(t, app, http.MethodGet, "/api/v1/admin/org-footer", "")
	if status != http.StatusOK {
		t.Fatalf("list status %d: %s", status, raw)
	}
	var rows []models.OrgFooter
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(rows) != 1 || rows[0].Domain != "t.example" {
		t.Fatalf("unexpected list: %+v", rows)
	}
}

func TestOrgFooterValidation(t *testing.T) {
	app, _ := newTestApp(t)
	cases := []struct {
		name   string
		body   string
		status int
	}{
		{"missing domain", `{"body_html":"<p>x</p>","enabled":true}`, http.StatusBadRequest},
		{"unknown domain", `{"domain":"nope.example","body_html":"<p>x</p>","enabled":true}`, http.StatusBadRequest},
		{"too large", `{"domain":"t.example","enabled":true,"body_html":"<p>` + strings.Repeat("x", maxFooterHTMLBytes+1) + `</p>"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := footerRequest(t, app, http.MethodPut, "/api/v1/admin/org-footer", tc.body)
			if status != tc.status {
				t.Fatalf("status %d want %d (%s)", status, tc.status, raw)
			}
		})
	}
}

func TestOrgFooterCannotBeEnabledEmpty(t *testing.T) {
	app, _ := newTestApp(t)
	status, raw := footerRequest(t, app, http.MethodPut, "/api/v1/admin/org-footer",
		`{"domain":"t.example","body_html":"","enabled":true}`)
	if status != http.StatusOK {
		t.Fatalf("save status %d: %s", status, raw)
	}
	var saved models.OrgFooter
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.Enabled {
		t.Fatalf("an empty footer must not be enabled: %+v", saved)
	}
}
