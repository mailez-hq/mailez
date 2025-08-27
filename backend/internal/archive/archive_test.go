package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
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

const sampleRaw = "From: Alice <alice@example.com>\r\n" +
	"To: bob@example.com, carol@example.com\r\n" +
	"Cc: dave@example.com\r\n" +
	"Subject: =?UTF-8?Q?=E6=B5=8B=E8=AF=95=E4=B8=BB=E9=A2=98?=\r\n" + // 测试主题 (encoded-word)
	"Message-ID: <abc123@example.com>\r\n" +
	"Date: Mon, 01 Jan 2024 10:00:00 +0800\r\n" +
	"\r\n" +
	"正文第一行\r\n第二行\r\n"

const sampleRawOutbound = "From: Bob <bob@example.com>\r\n" +
	"To: zed@remote.test\r\n" +
	"Subject: =?UTF-8?Q?=E6=B5=8B=E8=AF=95=E4=B8=BB=E9=A2=98?=\r\n" +
	"Message-ID: <abc123@example.com>\r\n" +
	"Date: Mon, 02 Jan 2024 11:00:00 +0800\r\n" +
	"\r\n" +
	"出站正文\r\n"

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "archive.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := core.New(db, auth.NewManager(db, auth.NewMemoryStore(), "s", time.Hour), core.Config{})
	return New(app)
}

func seedGlobal(t *testing.T, s *Service, enabled, inbound, outbound bool, retention int) {
	t.Helper()
	row := models.ArchiveSettings{Domain: "", Enabled: enabled, CaptureInbound: inbound, CaptureOutbound: outbound, RetentionDays: retention}
	// Select("*") forces zero-valued bools into the INSERT; the model's
	// default:true tags would otherwise turn an explicit false into true.
	if err := s.DB.Select("*").Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

func ingestRequest(t *testing.T, dir string, from string, to []string, raw string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	meta, _ := mw.CreateFormField("meta")
	_ = json.NewEncoder(meta).Encode(ingestEvent{
		Direction:    dir,
		EnvelopeFrom: from,
		EnvelopeTo:   to,
		ReceivedAt:   time.Now(),
	})
	rawPart, _ := mw.CreateFormFile("raw", "message.eml")
	_, _ = rawPart.Write([]byte(raw))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/stack/archive", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestIngestStoresAndParses(t *testing.T) {
	s := newTestService(t)
	seedGlobal(t, s, true, true, true, 0)
	f := fiber.New()
	s.RegisterStack(f.Group("/stack"))

	req := ingestRequest(t, "inbound", "alice@remote.test", []string{"bob@example.com"}, sampleRaw)
	resp, err := f.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}

	var row models.ArchivedMessage
	if err := s.DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Direction != models.ArchiveInbound {
		t.Errorf("direction = %q", row.Direction)
	}
	if row.Domain != "example.com" {
		t.Errorf("domain = %q", row.Domain)
	}
	if row.Subject != "测试主题" {
		t.Errorf("subject decoded = %q", row.Subject)
	}
	if row.From == "" || !strings.Contains(row.To, "bob@example.com") || !strings.Contains(row.Cc, "dave@example.com") {
		t.Errorf("headers: from=%q to=%q cc=%q", row.From, row.To, row.Cc)
	}
	if row.MessageID != "abc123@example.com" {
		t.Errorf("message id = %q", row.MessageID)
	}
	if row.Date.IsZero() {
		t.Error("date not parsed")
	}
	if row.ExpiresAt != nil {
		t.Errorf("retention 0 should keep forever, got %v", row.ExpiresAt)
	}
	if len(row.Raw) == 0 {
		t.Error("raw not stored")
	}
}

func TestIngestPolicyGates(t *testing.T) {
	s := newTestService(t)
	// Global off + domain on: only the domain captures.
	seedGlobal(t, s, false, true, true, 0)
	if err := s.DB.Select("*").Create(&models.ArchiveSettings{
		Domain: "example.com", Enabled: true, CaptureInbound: false, CaptureOutbound: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	f := fiber.New()
	s.RegisterStack(f.Group("/stack"))

	// Inbound to the domain: skipped (domain overrides capture_inbound=false).
	resp, _ := f.Test(ingestRequest(t, "inbound", "x@y.test", []string{"bob@example.com"}, sampleRaw), -1)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("inbound skip status = %d", resp.StatusCode)
	}
	// Outbound from the domain: captured.
	resp, _ = f.Test(ingestRequest(t, "outbound", "bob@example.com", []string{"x@y.test"}, sampleRaw), -1)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("outbound status = %d: %s", resp.StatusCode, b)
	}
	var count int64
	s.DB.Model(&models.ArchivedMessage{}).Count(&count)
	if count != 1 {
		t.Errorf("stored %d messages, want 1", count)
	}
}

func TestIngestAppliesRetention(t *testing.T) {
	s := newTestService(t)
	seedGlobal(t, s, true, true, true, 30)
	f := fiber.New()
	s.RegisterStack(f.Group("/stack"))

	resp, _ := f.Test(ingestRequest(t, "inbound", "x@y.test", []string{"bob@example.com"}, sampleRaw), -1)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var row models.ArchivedMessage
	if err := s.DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ExpiresAt == nil {
		t.Fatal("expected expiry")
	}
	want := row.ArchivedAt.AddDate(0, 0, 30)
	if row.ExpiresAt.Sub(want) > time.Minute || row.ExpiresAt.Before(want.Add(-time.Minute)) {
		t.Errorf("expires = %v, want ~%v", row.ExpiresAt, want)
	}
}

func TestListMessagesFilters(t *testing.T) {
	s := newTestService(t)
	seedGlobal(t, s, true, true, true, 0)
	store := func(dir, from string, to []string, raw string) {
		f := fiber.New()
		s.RegisterStack(f.Group("/stack"))
		resp, err := f.Test(ingestRequest(t, dir, from, to, raw), -1)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("ingest %s/%s: %d %v", dir, from, resp.StatusCode, err)
		}
	}
	store("inbound", "a@remote.test", []string{"bob@example.com"}, sampleRaw)
	store("outbound", "bob@example.com", []string{"z@remote.test"}, sampleRawOutbound)

	f := fiber.New()
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "admin@example.com", GlobalAdmin: true})
		return c.Next()
	})
	s.Register(authed)

	check := func(qs string, want int) {
		t.Helper()
		resp, err := f.Test(httptest.NewRequest(http.MethodGet, "/api/v1/archive/messages?"+qs, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Total int `json:"total"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if out.Total != want {
			t.Errorf("filter %q: total = %d, want %d", qs, out.Total, want)
		}
	}
	check("direction=inbound", 1)
	check("direction=outbound", 1)
	check("domain=example.com", 2)
	check("from=remote.test", 1)
	check("to=bob@example.com", 1)
	check("q=测试主题", 2)
	check("q=abc123", 2)
}

func TestRetentionWorkerPurgesExpired(t *testing.T) {
	s := newTestService(t)
	seedGlobal(t, s, true, true, true, 1)
	f := fiber.New()
	s.RegisterStack(f.Group("/stack"))
	resp, _ := f.Test(ingestRequest(t, "inbound", "x@y.test", []string{"bob@example.com"}, sampleRaw), -1)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	// Force the row into the past so the worker purges it.
	if err := s.DB.Model(&models.ArchivedMessage{}).
		Where("1 = 1").
		Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.RunRetention(ctx)
		close(done)
	}()
	// RunRetention purges synchronously on start, then blocks on the ticker;
	// poll until the row disappears.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		s.DB.Model(&models.ArchivedMessage{}).Count(&count)
		if count == 0 {
			cancel()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expired message not purged")
}
