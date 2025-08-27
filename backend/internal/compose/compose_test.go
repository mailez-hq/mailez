package compose

import (
	"encoding/base64"
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
	"mailez/backend/internal/mail"
)

// fakeGateway embeds the production gateway interface and overrides only the
// compose surface, so handler logic is tested without live mail services.
type fakeGateway struct {
	mail.Gateway
	sentTo     []string
	sentCc     []string
	sentAttach []mail.Attachment
	savedTo    []string
	savedUID   uint32
}

func (f *fakeGateway) With(dial mail.Dial) mail.Gateway { return f }

func (f *fakeGateway) Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, extra ...mail.Header) error {
	f.sentTo = to
	f.sentCc = cc
	f.sentAttach = attachments
	return nil
}

func (f *fakeGateway) SaveDraft(email, token string, to, cc []string, subject, text, html string, attachments []mail.Attachment, replaceUID uint32) (uint32, error) {
	f.savedTo = to
	f.savedUID = 99
	return f.savedUID, nil
}

func newTestApp(t *testing.T, gw mail.Gateway) (*fiber.App, *fakeGateway) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "compose.db")), &gorm.Config{
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
	if err := db.Create(&models.User{Email: "a@example.com", Localpart: "a", DomainName: "example.com", Enabled: true, GlobalAdmin: true}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret", Domain: "example.com"})
	fake, _ := gw.(*fakeGateway)
	app.Mail = fake

	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	h.Register(authed)
	return f, fake
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestMailSendWithAttachment(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{})
	att := base64.StdEncoding.EncodeToString([]byte("hello"))
	body, _ := json.Marshal(map[string]any{
		"to":      []string{"b@example.com"},
		"cc":      []string{"c@example.com"},
		"subject": "hi",
		"body":    "hello world",
		"attachments": []map[string]any{{
			"filename": "report.txt", "content_type": "text/plain", "data": att,
		}},
	})
	resp, _ := doJSON(t, app, http.MethodPost, "/api/v1/mail/send", string(body))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("send status = %d, want 204", resp.StatusCode)
	}
	if len(fake.sentTo) != 1 || fake.sentTo[0] != "b@example.com" {
		t.Errorf("sentTo = %v", fake.sentTo)
	}
	if len(fake.sentCc) != 1 || fake.sentCc[0] != "c@example.com" {
		t.Errorf("sentCc = %v", fake.sentCc)
	}
	if len(fake.sentAttach) != 1 || fake.sentAttach[0].Filename != "report.txt" {
		t.Errorf("sentAttach = %+v", fake.sentAttach)
	}
}

func TestMailSendForbiddenIdentity(t *testing.T) {
	app, _ := newTestApp(t, &fakeGateway{})
	body, _ := json.Marshal(map[string]any{
		"from": "stranger@example.com",
		"to":   []string{"b@example.com"},
	})
	resp, _ := doJSON(t, app, http.MethodPost, "/api/v1/mail/send", string(body))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("send-as stranger status = %d, want 403", resp.StatusCode)
	}
}

func TestMailSaveDraft(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{})
	body, _ := json.Marshal(map[string]any{
		"to":      []string{"b@example.com"},
		"subject": "draft",
		"text":    "body",
	})
	resp, out := doJSON(t, app, http.MethodPost, "/api/v1/mail/draft", string(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("draft status = %d, want 200", resp.StatusCode)
	}
	var got map[string]uint32
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["uid"] != 99 {
		t.Errorf("draft response = %s, want uid 99", out)
	}
	if len(fake.savedTo) != 1 || fake.savedTo[0] != "b@example.com" {
		t.Errorf("savedTo = %v", fake.savedTo)
	}
}
