//go:build mailez_ee

package admin

import (
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mailez/backend/internal/core/models"
)

func TestAuditExportCSV(t *testing.T) {
	app, db := newTestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := []models.AuditLog{
		{CreatedAt: now, User: "a@example.com", IP: "10.0.0.1", Method: "POST", Path: "/api/v1/users", Status: 200, Action: "users.create", Target: "bob@example.com"},
		{CreatedAt: now.Add(-time.Hour), User: "legacy@example.com", IP: "10.0.0.2", Method: "PUT", Path: "/api/v1/branding", Status: 200},
		{CreatedAt: now.Add(-2 * time.Hour), User: "=evil@example.com", IP: "10.0.0.3", Method: "DELETE", Path: "/api/v1/users/x", Status: 403, Detail: "@cmd"},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/audit/export", nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type = %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("content-disposition = %q, want attachment", cd)
	}
	rows, err := csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("csv rows = %d, want 4 (header + 3)", len(rows))
	}
	if got := strings.Join(rows[0], ","); got != "id,created_at,user,ip,method,path,status,action,target,detail" {
		t.Fatalf("header = %q", got)
	}
	// Rows are id-ascending, i.e. in seed order.
	if rows[1][7] != "users.create" || rows[1][8] != "bob@example.com" {
		t.Fatalf("enriched row = %v", rows[1])
	}
	if rows[2][7] != "" {
		t.Fatalf("legacy row = %v", rows[2])
	}
	if rows[3][2] != "'=evil@example.com" || rows[3][9] != "'@cmd" {
		t.Fatalf("formula-guard row = %v", rows[3])
	}
}

func TestAuditExportFilters(t *testing.T) {
	app, db := newTestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	rows := []models.AuditLog{
		{CreatedAt: now, User: "a@example.com", Method: "POST", Path: "/x", Status: 200},
		{CreatedAt: now.Add(-48 * time.Hour), User: "a@example.com", Method: "POST", Path: "/y", Status: 200},
		{CreatedAt: now, User: "other@example.com", Method: "POST", Path: "/z", Status: 200},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	// user filter
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/audit/export?user=other%40example.com", nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got, err := csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 || got[1][2] != "other@example.com" {
		t.Fatalf("user filter rows = %v", got)
	}

	// from filter (RFC3339, one hour ago)
	from := now.Add(-time.Hour).Format(time.RFC3339)
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/audit/export?from="+strings.ReplaceAll(from, ":", "%3A"), nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got, err = csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("from filter rows = %d, want 3", len(got))
	}

	// bad from
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/audit/export?from=nonsense", nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad from status = %d, want 400", resp.StatusCode)
	}
}
