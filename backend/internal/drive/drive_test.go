package drive

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func newDriveApp(t *testing.T) (*fiber.App, *Service) {
	t.Helper()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "drive.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.DriveFile{}); err != nil {
		t.Fatal(err)
	}
	svc, err := New(db, core.Config{SecretKey: "test-secret", UploadDir: filepath.Join(dir, "uploads")})
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	authed := app.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "alice@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	svc.Register(authed)
	return app, svc
}

func driveBody(t *testing.T, name, content string, parentID string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("parent_id", parentID)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(content))
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func TestDriveLifecycle(t *testing.T) {
	app, _ := newDriveApp(t)
	// Create a folder.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drive/folders", strings.NewReader(`{"name":"文档"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"文档"`) {
		t.Fatalf("folder create: %d %s", resp.StatusCode, b)
	}
	// Upload into the folder.
	body, ct := driveBody(t, "note.txt", "hello drive", "1")
	upReq := httptest.NewRequest(http.MethodPost, "/api/v1/drive/upload", body)
	upReq.Header.Set("Content-Type", ct)
	upResp, err := app.Test(upReq)
	if err != nil {
		t.Fatal(err)
	}
	upB, _ := io.ReadAll(upResp.Body)
	upResp.Body.Close()
	if upResp.StatusCode != http.StatusOK {
		t.Fatalf("upload: %d %s", upResp.StatusCode, upB)
	}
	// List the folder: one file.
	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/drive/tree?parent_id=1", nil))
	if err != nil {
		t.Fatal(err)
	}
	listB, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if !strings.Contains(string(listB), "note.txt") {
		t.Fatalf("list: %s", listB)
	}
	// Owner download returns the content.
	dlResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/drive/download/2", nil))
	if err != nil {
		t.Fatal(err)
	}
	dlB, _ := io.ReadAll(dlResp.Body)
	dlResp.Body.Close()
	if string(dlB) != "hello drive" {
		t.Fatalf("download content = %q", dlB)
	}
	// Trash and restore.
	trashReq := httptest.NewRequest(http.MethodPost, "/api/v1/drive/trash", strings.NewReader(`{"ids":[2]}`))
	trashReq.Header.Set("Content-Type", "application/json")
	tr, _ := app.Test(trashReq)
	tr.Body.Close()
	if tr.StatusCode != http.StatusNoContent {
		t.Fatalf("trash status = %d", tr.StatusCode)
	}
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/drive/restore", strings.NewReader(`{"ids":[2]}`))
	restoreReq.Header.Set("Content-Type", "application/json")
	rr, _ := app.Test(restoreReq)
	rr.Body.Close()
	if rr.StatusCode != http.StatusNoContent {
		t.Fatalf("restore status = %d", rr.StatusCode)
	}
}
