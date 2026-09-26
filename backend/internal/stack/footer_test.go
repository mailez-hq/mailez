package stack

import (
	"encoding/json"
	"net/http"
	"testing"

	"mailez/backend/internal/core/models"
)

func TestOrgFooterLookup(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)
	if err := h.DB.Create(&models.OrgFooter{
		Domain: "example.com", BodyHTML: "<p>ACME Confidential</p>", BodyText: "ACME Confidential", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("seed footer: %v", err)
	}
	if err := h.DB.Create(&models.OrgFooter{
		Domain: "off.example.com", BodyHTML: "<p>off</p>", BodyText: "off", Enabled: false,
	}).Error; err != nil {
		t.Fatalf("seed disabled footer: %v", err)
	}

	code, body := doGet(t, app, "/stack/org-footer?domain=example.com")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var got struct {
		Domain   string `json:"domain"`
		Enabled  bool   `json:"enabled"`
		BodyHTML string `json:"body_html"`
		BodyText string `json:"body_text"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if !got.Enabled || got.Domain != "example.com" || got.BodyHTML != "<p>ACME Confidential</p>" || got.BodyText != "ACME Confidential" {
		t.Fatalf("unexpected payload: %+v", got)
	}

	for _, path := range []string{
		"/stack/org-footer?domain=EXAMPLE.com",
		"/stack/org-footer?domain=off.example.com",
		"/stack/org-footer?domain=nope.example",
	} {
		code, body := doGet(t, app, path)
		if code != http.StatusOK {
			t.Fatalf("%s: status %d (%s)", path, code, body)
		}
		if path == "/stack/org-footer?domain=EXAMPLE.com" {
			continue
		}
		var envelope struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal([]byte(body), &envelope); err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		if envelope.Enabled {
			t.Fatalf("%s: footer must not be enabled: %s", path, body)
		}
	}

	if code, _ := doGet(t, app, "/stack/org-footer"); code != http.StatusBadRequest {
		t.Fatalf("missing domain: status %d, want 400", code)
	}
}
