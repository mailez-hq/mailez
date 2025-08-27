package mailbox

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

type zipFake struct {
	fakeGateway
	msg *mail.Message
}

func (f *zipFake) With(dial mail.Dial) mail.Gateway { return f }

func (f *zipFake) GetMessage(email, token, folder string, uid uint32) (*mail.Message, error) {
	return f.msg, nil
}

func TestMailAttachmentsZip(t *testing.T) {
	fake := &zipFake{
		msg: &mail.Message{
			UID: 7, Subject: "季度报告: 2026",
			Attachments: []mail.Attachment{
				{Filename: "a.txt", ContentType: "text/plain", Data: base64.StdEncoding.EncodeToString([]byte("aaa"))},
				{Filename: "b.txt", ContentType: "text/plain", Data: base64.StdEncoding.EncodeToString([]byte("bbb"))},
			},
		},
	}
	app := zipApp(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail/attachments/zip?folder=Inbox&uid=7", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content type = %q", ct)
	}
	raw, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("zip read: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip entries = %d", len(zr.File))
	}
	var buf bytes.Buffer
	rc, _ := zr.File[0].Open()
	_, _ = io.Copy(&buf, rc)
	rc.Close()
	if buf.String() != "aaa" {
		t.Fatalf("first entry = %q", buf.String())
	}
}

func zipApp(t *testing.T, gw *zipFake) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "zip.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
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

var _ mail.Gateway = (*zipFake)(nil)
