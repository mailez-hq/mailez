package alias

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func newAliasTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "aliases.db")), &gorm.Config{
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
	if err := db.Create(&models.Domain{Name: "t.example", MaxUsers: 10, MaxAliases: 10}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	h.Register(authed)
	return f
}

func TestAliasCRUD(t *testing.T) {
	app := newAliasTestApp(t)
	body := `{"email":"team@t.example","destination":"a@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aliases", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/aliases", nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	b, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	var page struct {
		Data  []models.Alias `json:"data"`
		Total int            `json:"total"`
	}
	if err := json.Unmarshal(b, &page); err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].Email != "team@t.example" {
		t.Fatalf("list = %s", string(b))
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/aliases/team@t.example", nil)
	if resp, _ := app.Test(del, -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
}

func TestAliasDistributionGroupMembers(t *testing.T) {
	app := newAliasTestApp(t)
	body := `{"email":"sales@t.example","name":"Sales Team","destination":"a@example.com","members":[{"email":"Bob@Example.com","name":"Bob"},{"email":"carol@example.com"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aliases", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status = %d, body %s", resp.StatusCode, b)
	}

	// GET one: members come back as a parsed array, emails normalized.
	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/aliases/sales@t.example", nil))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getResp.Body.Close()
	var got struct {
		Email    string                `json:"email"`
		Name     string                `json:"name"`
		Members  []models.AliasMember  `json:"members"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Name != "Sales Team" || len(got.Members) != 2 {
		t.Fatalf("get = %+v", got)
	}
	if got.Members[0].Email != "bob@example.com" {
		t.Fatalf("member email not normalized: %+v", got.Members[0])
	}

	// PUT replaces the member list (and keeps name/destination).
	putBody := `{"name":"Sales & Marketing","members":[{"email":"dave@example.com"}]}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/aliases/sales@t.example", strings.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := app.Test(putReq, -1)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("put status = %d, body %s", putResp.StatusCode, b)
	}
	var updated struct {
		Name       string                `json:"name"`
		Destination string               `json:"destination"`
		Members    []models.AliasMember  `json:"members"`
	}
	if err := json.NewDecoder(putResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode put: %v", err)
	}
	if updated.Name != "Sales & Marketing" || updated.Destination != "a@example.com" ||
		len(updated.Members) != 1 || updated.Members[0].Email != "dave@example.com" {
		t.Fatalf("put result = %+v", updated)
	}

	// Invalid member address is rejected.
	bad := `{"email":"bad@t.example","members":[{"email":"no-at-sign"}]}`
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/aliases", strings.NewReader(bad))
	badReq.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(badReq, -1); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad member status = %d, want 400", resp.StatusCode)
	}

	// A members-only group (no destination) is valid.
	only := `{"email":"hr@t.example","members":[{"email":"amy@example.com"}]}`
	onlyReq := httptest.NewRequest(http.MethodPost, "/api/v1/aliases", strings.NewReader(only))
	onlyReq.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(onlyReq, -1); resp.StatusCode != http.StatusCreated {
		t.Fatalf("members-only group status = %d, want 201", resp.StatusCode)
	}

	// An alias with neither destination nor members is rejected.
	empty := `{"email":"empty@t.example"}`
	emptyReq := httptest.NewRequest(http.MethodPost, "/api/v1/aliases", strings.NewReader(empty))
	emptyReq.Header.Set("Content-Type", "application/json")
	if resp, _ := app.Test(emptyReq, -1); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty alias status = %d, want 400", resp.StatusCode)
	}
}

// TestAliasEncodedPathParam pins the %40-encoded email path handling used by
// the admin UI (encodeURIComponent), which must resolve to the same alias as
// the raw address.
func TestAliasEncodedPathParam(t *testing.T) {
	app := newAliasTestApp(t)
	body := `{"email":"sales@t.example","destination":"a@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aliases", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(req, -1); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %v status %d", err, resp.StatusCode)
	}

	enc := url.PathEscape("sales@t.example")
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/aliases/"+enc, nil)
	if resp, err := app.Test(getReq, -1); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get encoded: %v status %d", err, resp.StatusCode)
	}
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/aliases/"+enc, strings.NewReader(`{"destination":"b@example.com"}`))
	putReq.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(putReq, -1); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("put encoded: %v status %d", err, resp.StatusCode)
	}
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/aliases/"+enc, nil)
	if resp, err := app.Test(delReq, -1); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete encoded: %v status %d", err, resp.StatusCode)
	}
}
