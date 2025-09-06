//go:build mailez_ee

package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBrandingRoundtrip is enterprise-only: branding is an EE feature, the
// community build does not mount the routes at all.
func TestBrandingRoundtrip(t *testing.T) {
	app, _ := newTestApp(t)

	// Fresh instance returns empty defaults.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil))
	if err != nil {
		t.Fatalf("branding get: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var empty map[string]any
	if err := json.Unmarshal(b, &empty); err != nil {
		t.Fatalf("branding json: %v", err)
	}
	if title, _ := empty["title"].(string); title != "" {
		t.Fatalf("initial title = %q, want empty", title)
	}

	// Save a custom brand.
	put := httptest.NewRequest(http.MethodPut, "/api/v1/branding", strings.NewReader(`{
		"title": "Example University",
		"subtitle": "电子邮件系统",
		"tagline": "安全、稳定、高效",
		"feature1": "特性一",
		"feature2": "特性二",
		"feature3": "特性三",
		"logo_url": "https://example.com/logo.png",
		"hero_url": "https://example.com/hero.jpg",
		"copyright": "Copyright © example.com, All Rights Reserved",
		"contact": "support@example.com",
		"website_url": "https://www.example.com"
	}`))
	put.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(put)
	if err != nil {
		t.Fatalf("branding put: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("branding put status = %d, body %s", resp.StatusCode, string(b))
	}

	// Read it back.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil))
	if err != nil {
		t.Fatalf("branding get after put: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("branding json after put: %v", err)
	}
	if got["title"] != "Example University" {
		t.Fatalf("title after put = %v, want Example University", got["title"])
	}
	if got["logo_url"] != "https://example.com/logo.png" {
		t.Fatalf("logo_url after put = %v", got["logo_url"])
	}
	if got["hero_url"] != "https://example.com/hero.jpg" {
		t.Fatalf("hero_url after put = %v", got["hero_url"])
	}
	if got["contact"] != "support@example.com" {
		t.Fatalf("contact after put = %v", got["contact"])
	}
	if got["website_url"] != "https://www.example.com" {
		t.Fatalf("website_url after put = %v", got["website_url"])
	}
}
