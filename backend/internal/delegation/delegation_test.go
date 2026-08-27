package delegation

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

func newDelegationHarness(t *testing.T, user *models.User) (*Handler, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "delegation.db")), &gorm.Config{
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
	h := New(core.New(db, mgr, core.Config{SecretKey: "test-secret"}))
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", user)
		return c.Next()
	})
	h.Register(app.Group("/api/v1"))
	return h, app
}

func seedUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, u := range []*models.User{
		{Email: "alice@example.com", Localpart: "alice", DomainName: "example.com", Enabled: true, DisplayedName: "Alice"},
		{Email: "bob@example.com", Localpart: "bob", DomainName: "example.com", Enabled: true, DisplayedName: "Bob"},
	} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("seed %s: %v", u.Email, err)
		}
	}
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestDelegationCRUD(t *testing.T) {
	alice := &models.User{Email: "alice@example.com", Localpart: "alice", DomainName: "example.com", Enabled: true}
	h, app := newDelegationHarness(t, alice)
	seedUsers(t, h.DB)

	// Invalid: self-delegation and unknown delegates.
	if code, body := doJSON(t, app, http.MethodPost, "/api/v1/delegations", `{"delegate_email":"alice@example.com","can_send":true}`); code != 400 {
		t.Fatalf("self-delegation: %d %s", code, body)
	}
	if code, _ := doJSON(t, app, http.MethodPost, "/api/v1/delegations", `{"delegate_email":"nobody@example.com","can_send":true}`); code != 400 {
		t.Fatalf("unknown delegate accepted: %d", code)
	}
	// Invalid: no permission selected.
	if code, _ := doJSON(t, app, http.MethodPost, "/api/v1/delegations", `{"delegate_email":"bob@example.com","can_send":false,"full_access":false}`); code != 400 {
		t.Fatalf("empty permission accepted: %d", code)
	}

	// Create a full-access grant (implies can_send).
	code, body := doJSON(t, app, http.MethodPost, "/api/v1/delegations", `{"delegate_email":"BOB@example.com","can_send":false,"full_access":true}`)
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	var created View
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
	if !created.FullAccess || !created.CanSend || created.DelegateEmail != "bob@example.com" || created.DelegateName != "Bob" {
		t.Fatalf("created view: %+v", created)
	}

	// Duplicate grant is rejected.
	if code, _ := doJSON(t, app, http.MethodPost, "/api/v1/delegations", `{"delegate_email":"bob@example.com","can_send":true}`); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}

	// List shows granted and received sides.
	code, body = doJSON(t, app, http.MethodGet, "/api/v1/delegations", "")
	if code != 200 {
		t.Fatalf("list: %d %s", code, body)
	}
	var listing struct {
		Granted  []View `json:"granted"`
		Received []View `json:"received"`
	}
	if err := json.Unmarshal([]byte(body), &listing); err != nil {
		t.Fatalf("unmarshal list %s: %v", body, err)
	}
	if len(listing.Granted) != 1 || len(listing.Received) != 0 {
		t.Fatalf("listing: %+v", listing)
	}

	// Downgrade to send-only.
	code, body = doJSON(t, app, http.MethodPut, "/api/v1/delegations/1", `{"can_send":true,"full_access":false}`)
	if code != 200 {
		t.Fatalf("update: %d %s", code, body)
	}
	var updated View
	if err := json.Unmarshal([]byte(body), &updated); err != nil {
		t.Fatalf("unmarshal update %s: %v", body, err)
	}
	if updated.FullAccess || !updated.CanSend {
		t.Fatalf("updated view: %+v", updated)
	}

	// Revoke.
	if code, _ := doJSON(t, app, http.MethodDelete, "/api/v1/delegations/1", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := doJSON(t, app, http.MethodGet, "/api/v1/delegations", ""); code != 200 {
		t.Fatalf("list after delete: %d", code)
	}
}

// TestDelegationMailDial verifies the full-access header switches the mailbox
// context of every /mail/* request, and that send-only grants cannot open the
// mailbox.
func TestDelegationMailDial(t *testing.T) {
	bob := &models.User{Email: "bob@example.com", Localpart: "bob", DomainName: "example.com", Enabled: true}
	h, app := newDelegationHarness(t, bob)
	seedUsers(t, h.DB)

	var dialEmail string
	var dialErr string
	app.Post("/api/v1/_dial", func(c *fiber.Ctx) error {
		d, err := h.App.MailDial(c)
		if err != nil {
			dialErr = err.Error()
			return c.Status(500).SendString(dialErr)
		}
		dialEmail = d.Email
		return c.SendStatus(204)
	})

	// Send-only grant: mailbox access is refused.
	if err := h.DB.Create(&models.MailDelegation{
		OwnerEmail: "alice@example.com", DelegateEmail: "bob@example.com",
		CanSend: true, FullAccess: false,
	}).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/_dial", nil)
	req.Header.Set("X-Delegate-Email", "alice@example.com")
	if resp, err := app.Test(req, 5000); err != nil || resp.StatusCode != 500 {
		t.Fatalf("send-only dial: %v %v", err, resp)
	} else {
		resp.Body.Close()
	}
	if dialErr == "" {
		t.Fatal("expected authorization error for send-only grant")
	}

	// Full access: the dial targets alice's mailbox.
	if err := h.DB.Model(&models.MailDelegation{}).
		Where("owner_email = ? AND delegate_email = ?", "alice@example.com", "bob@example.com").
		Update("full_access", true).Error; err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/_dial", nil)
	req.Header.Set("X-Delegate-Email", "alice@example.com")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 || dialEmail != "alice@example.com" {
		t.Fatalf("full-access dial: %d email=%q", resp.StatusCode, dialEmail)
	}
}
