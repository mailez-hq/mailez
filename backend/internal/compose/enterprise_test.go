package compose

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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

// enterpriseFake implements the mail surface the enterprise handlers use.
type enterpriseFake struct {
	mail.Gateway
	mu sync.Mutex

	rawByUID map[uint32]string
	folders  []string
	uidByID  map[string]uint32

	sentTo   []string
	sentSubj string
	sentText string
	sentHdrs []mail.Header
	flags    map[uint32][]string
	deleted  []string
}

func newEnterpriseFake() *enterpriseFake {
	return &enterpriseFake{
		rawByUID: map[uint32]string{},
		folders:  []string{"Inbox", "Sent"},
		uidByID:  map[string]uint32{},
		flags:    map[uint32][]string{},
	}
}

func (f *enterpriseFake) With(dial mail.Dial) mail.Gateway { return f }

func (f *enterpriseFake) GetRaw(email, token, folder string, uid uint32) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rawByUID[uid], nil
}

func (f *enterpriseFake) SetFlag(email, token, folder string, uid uint32, flag string, value bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value {
		f.flags[uid] = append(f.flags[uid], flag)
	} else {
		var out []string
		for _, x := range f.flags[uid] {
			if x != flag {
				out = append(out, x)
			}
		}
		f.flags[uid] = out
	}
	return nil
}

func (f *enterpriseFake) ListFolders(email, token string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.folders...), nil
}

func (f *enterpriseFake) UIDByMessageID(email, token, folder, id string) (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	uid, ok := f.uidByID[id]
	if !ok {
		return 0, errNotFound
	}
	return uid, nil
}

func (f *enterpriseFake) Delete(email, token, folder string, uid uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, folder)
	return nil
}

func (f *enterpriseFake) Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, extra ...mail.Header) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentTo = append([]string(nil), to...)
	f.sentSubj = subject
	f.sentText = text
	f.sentHdrs = append([]mail.Header(nil), extra...)
	return nil
}

var errNotFound = &fakeErr{}

type fakeErr struct{}

func (e *fakeErr) Error() string { return "not found" }

func newEnterpriseApp(t *testing.T, gw *enterpriseFake) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "compose-e.db")), &gorm.Config{
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
	app := core.New(db, mgr, core.Config{SecretKey: "test-secret"})
	app.Mail = gw
	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	h.Register(authed)
	return f
}

func postJSON(t *testing.T, app *fiber.App, path, body string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestSendReceipt(t *testing.T) {
	gw := newEnterpriseFake()
	gw.rawByUID[42] = "Message-ID: <abc@example.com>\r\nSubject: 季度报告\r\n" +
		"Disposition-Notification-To: boss@example.com\r\n\r\nbody"
	app := newEnterpriseApp(t, gw)
	resp, _ := postJSON(t, app, "/api/v1/mail/receipt", `{"folder":"Inbox","uid":42}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(gw.sentTo) != 1 || gw.sentTo[0] != "boss@example.com" {
		t.Fatalf("receipt recipient = %v", gw.sentTo)
	}
	if !strings.HasPrefix(gw.sentSubj, "已读回执") {
		t.Fatalf("receipt subject = %q", gw.sentSubj)
	}
	found := false
	for _, h := range gw.sentHdrs {
		if h.Key == "X-MDN-Disposition" && strings.Contains(h.Value, "displayed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing MDN header: %+v", gw.sentHdrs)
	}
	if !contains(gw.flags[42], `$MDNSent`) {
		t.Fatalf("$MDNSent not set: %v", gw.flags[42])
	}
}

func TestRecallAndApply(t *testing.T) {
	gw := newEnterpriseFake()
	gw.rawByUID[7] = "Message-ID: <orig@example.com>\r\nSubject: 项目方案\r\n" +
		"To: bob@example.com, Carol <carol@example.com>\r\n\r\nbody"
	app := newEnterpriseApp(t, gw)

	resp, body := postJSON(t, app, "/api/v1/mail/recall", `{"folder":"Sent","uid":7}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recall status = %d: %s", resp.StatusCode, body)
	}
	if len(gw.sentTo) != 2 {
		t.Fatalf("recall recipients = %v", gw.sentTo)
	}
	hasRecall := false
	for _, h := range gw.sentHdrs {
		if h.Key == "X-MS-Recall" && h.Value == "yes" {
			hasRecall = true
		}
	}
	if !hasRecall {
		t.Fatalf("missing X-MS-Recall header: %+v", gw.sentHdrs)
	}
	if !contains(gw.flags[7], `$RecallSent`) {
		t.Fatalf("$RecallSent not set: %v", gw.flags[7])
	}

	// Apply the recall on the reader side: the original is found by id.
	gw.uidByID[mail.EncodeMessageID("<orig@example.com>")] = 11
	resp2, body2 := postJSON(t, app, "/api/v1/mail/recall/apply", `{"message_id":"<orig@example.com>"}`)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("recall apply status = %d: %s", resp2.StatusCode, body2)
	}
	var out struct {
		Removed int `json:"removed"`
	}
	if err := json.Unmarshal([]byte(body2), &out); err != nil || out.Removed != 1 {
		t.Fatalf("recall apply result = %s", body2)
	}
	if len(gw.deleted) == 0 || gw.deleted[0] != "Inbox" {
		t.Fatalf("original not deleted: %v", gw.deleted)
	}
}

func TestMailMerge(t *testing.T) {
	gw := newEnterpriseFake()
	app := newEnterpriseApp(t, gw)
	payload := `{
		"subject": "通知：{{name}}（{{email}}）",
		"body": "您好 {{name}}，您的工号是 {{staff_id}}。",
		"recipients": [
			{"email": "bob@example.com", "name": "Bob", "vars": {"staff_id": "1001"}},
			{"email": "carol@example.com", "name": "Carol", "vars": {"staff_id": "1002"}}
		]
	}`
	resp, body := postJSON(t, app, "/api/v1/mail/merge", payload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Sent   int `json:"sent"`
		Failed []map[string]any
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("merge result: %v", err)
	}
	if out.Sent != 2 {
		t.Fatalf("merge sent = %d", out.Sent)
	}
	// The fake keeps only the last send; verify substitution happened for it.
	if !strings.Contains(gw.sentText, "Carol") || !strings.Contains(gw.sentText, "1002") {
		t.Fatalf("merge body not substituted: %q", gw.sentText)
	}
	if len(gw.sentTo) != 1 || gw.sentTo[0] != "carol@example.com" {
		t.Fatalf("merge per-recipient To = %v", gw.sentTo)
	}
}

func TestSubstitute(t *testing.T) {
	got := substitute("Hi {{Name}} / {{ staff_id }}", map[string]string{"name": "Bob", "staff_id": "7"})
	if got != "Hi Bob / 7" {
		t.Fatalf("substitute = %q", got)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
