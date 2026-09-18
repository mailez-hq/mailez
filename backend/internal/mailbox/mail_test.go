package mailbox

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
	"mailez/backend/internal/mail"
)

type fakeGateway struct {
	mail.Gateway
	messages []mail.Message
	total    int
	moved    [][2]string
	marked   []string
	flags    []string
	purged   []purgeCall
}

type purgeCall struct {
	folder string
	uids   []uint32
}

func (f *fakeGateway) With(dial mail.Dial) mail.Gateway { return f }

func (f *fakeGateway) Purge(email, token, folder string, uid uint32) error {
	return f.PurgeMany(email, token, folder, []uint32{uid})
}

func (f *fakeGateway) PurgeMany(email, token, folder string, uids []uint32) error {
	f.purged = append(f.purged, purgeCall{folder: folder, uids: append([]uint32(nil), uids...)})
	return nil
}

func (f *fakeGateway) ListMessages(email, token, folder string, page int) ([]mail.Message, int, error) {
	return f.messages, f.total, nil
}

func (f *fakeGateway) ListMessagesSorted(email, token, folder string, page int, sort, dir string) ([]mail.Message, int, error) {
	return f.messages, f.total, nil
}

func (f *fakeGateway) MoveMany(email, token, folder string, uids []uint32, destination string) error {
	f.moved = append(f.moved, [2]string{folder, destination})
	return nil
}

func (f *fakeGateway) MarkAllRead(email, token, folder string) error {
	f.marked = append(f.marked, folder)
	return nil
}

func (f *fakeGateway) SetFlag(email, token, folder string, uid uint32, flag string, value bool) error {
	f.flags = append(f.flags, flag)
	return nil
}

func newTestApp(t *testing.T, gw mail.Gateway) (*fiber.App, *fakeGateway) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mailbox.db")), &gorm.Config{
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
	fake, _ := gw.(*fakeGateway)
	app.Mail = fake

	h := New(app)
	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	h.Register(authed)
	return f, fake
}

func TestMailMessages(t *testing.T) {
	app, _ := newTestApp(t, &fakeGateway{
		messages: []mail.Message{{UID: 1, Subject: "hello"}},
		total:    3,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail/messages?folder=INBOX", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Total-Messages"); got != "3" {
		t.Errorf("X-Total-Messages = %q, want 3", got)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var msgs []mail.Message
	if err := json.Unmarshal(b, &msgs); err != nil || len(msgs) != 1 || msgs[0].Subject != "hello" {
		t.Errorf("body = %s, want one 'hello' message", string(b))
	}
}

func TestMailMove(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{})
	body := `{"folder":"INBOX","uid":5,"destination":"Archive"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail/move", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if len(fake.moved) != 1 || fake.moved[0] != [2]string{"INBOX", "Archive"} {
		t.Errorf("moved = %v", fake.moved)
	}
}

// Deleting from an ordinary folder files the message under Trash.
func TestMailDeleteMovesToTrash(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail/delete", strings.NewReader(`{"folder":"INBOX","uid":5}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if len(fake.moved) != 1 || fake.moved[0] != [2]string{"INBOX", "Trash"} {
		t.Errorf("moved = %v, want INBOX → Trash", fake.moved)
	}
	if len(fake.purged) != 0 {
		t.Errorf("purged = %v, want none", fake.purged)
	}
}

// Regression: deleting inside Trash used to issue Trash → Trash, which the
// engine treats as a no-op, so the message stayed in the trash forever.
func TestMailDeleteInTrashPurges(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail/delete", strings.NewReader(`{"folder":"Trash","uids":[7,8]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if len(fake.purged) != 1 || fake.purged[0].folder != "Trash" || len(fake.purged[0].uids) != 2 {
		t.Fatalf("purged = %v, want one Trash call with two uids", fake.purged)
	}
	if len(fake.moved) != 0 {
		t.Errorf("moved = %v, want no move", fake.moved)
	}
}

// Regression: the /mail/flag API speaks display names too. A bare "seen"
// must not reach the IMAP wire verbatim: it would silently create a custom
// keyword that never touches the real \Seen state (unread badges never move).
func TestMailFlagCanonicalizesSystemFlags(t *testing.T) {
	app, fake := newTestApp(t, &fakeGateway{})
	post := func(body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mail/flag", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("flag %s: status = %d, want 204", body, resp.StatusCode)
		}
	}
	post(`{"folder":"INBOX","uid":7,"flag":"seen","value":true}`)
	post(`{"folder":"INBOX","uid":7,"flag":"\\Flagged","value":true}`)
	post(`{"folder":"INBOX","uid":7,"flag":"$MDNSent","value":true}`)
	want := []string{`\Seen`, `\Flagged`, "$MDNSent"}
	if len(fake.flags) != len(want) {
		t.Fatalf("captured flags = %v, want %v", fake.flags, want)
	}
	for i, w := range want {
		if fake.flags[i] != w {
			t.Errorf("flags[%d] = %q, want %q", i, fake.flags[i], w)
		}
	}
}
