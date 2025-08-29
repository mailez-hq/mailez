package uploads

import (
	"bytes"
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

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func newUploadApp(t *testing.T) (*fiber.App, *Service, *gorm.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "up.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.UploadedFile{}); err != nil {
		t.Fatal(err)
	}
	svc := New(db, core.Config{SecretKey: "test-secret", UploadDir: filepath.Join(dir, "uploads")})
	app := fiber.New()
	authed := app.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "alice@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	svc.Register(authed)
	return app, svc, db
}

func multipartBody(t *testing.T, name, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(content))
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func TestUploadDownloadDelete(t *testing.T) {
	app, svc, _ := newUploadApp(t)
	body, ct := multipartBody(t, "report.pdf", "pdf-bytes")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
	req.Header.Set("Content-Type", ct)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d: %s", resp.StatusCode, b)
	}
	var out struct {
		ID   uint   `json:"id"`
		URL  string `json:"url"`
		Name string `json:"filename"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.ID == 0 || !strings.Contains(out.URL, "token=") {
		t.Fatalf("upload result = %+v", out)
	}
	// Bad token rejected.
	bad := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/1/download?token=bad", nil)
	badResp, _ := app.Test(bad)
	if badResp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token status = %d", badResp.StatusCode)
	}
	badResp.Body.Close()
	// Good token streams the file.
	good := httptest.NewRequest(http.MethodGet, out.URL, nil)
	goodResp, err := app.Test(good)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(goodResp.Body)
	goodResp.Body.Close()
	if string(data) != "pdf-bytes" {
		t.Fatalf("download content = %q", data)
	}
	// Delete works for the owner.
	del := httptest.NewRequest(http.MethodDelete, "/api/v1/uploads/1", nil)
	delResp, _ := app.Test(del)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", delResp.StatusCode)
	}
	_ = svc
}

func TestCleanupRemovesExpired(t *testing.T) {
	_, svc, db := newUploadApp(t)
	expired := time.Now().Add(-time.Hour)
	if err := db.Create(&models.UploadedFile{
		UserEmail: "alice@example.com", Filename: "old.bin", StoredPath: "alice/old.bin",
		ExpiresAt: &expired,
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc.cleanup()
	var count int64
	db.Model(&models.UploadedFile{}).Count(&count)
	if count != 0 {
		t.Fatalf("expired uploads remain: %d", count)
	}
}
