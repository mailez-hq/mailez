package domain

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

func newDkimTestHandler(t *testing.T) (*Handler, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dkim.db")), &gorm.Config{
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
	cfg := core.Config{SecretKey: "test-secret", DkimSelector: "dkim"}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	h := New(core.New(db, mgr, cfg))
	app := fiber.New()
	// Register only the DKIM routes with a pass-through middleware; role/auth
	// enforcement is covered by the shared auth flow elsewhere.
	h.registerDkim(app.Group("/api/v1"), func(c *fiber.Ctx) error { return c.Next() })
	return h, app
}

func dkimDo(t *testing.T, app *fiber.App, method, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("unmarshal %q: %v", body, err)
		}
	}
	return resp.StatusCode, out
}

func TestDkimStatusAndGenerate(t *testing.T) {
	h, app := newDkimTestHandler(t)
	if err := h.DB.Create(&models.Domain{Name: "example.com"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	// No key yet.
	code, out := dkimDo(t, app, http.MethodGet, "/api/v1/domains/example.com/dkim")
	if code != 200 || out["enabled"] != false || out["public_key"] != "" {
		t.Fatalf("status: got %d %v", code, out)
	}

	// Generate.
	code, out = dkimDo(t, app, http.MethodPost, "/api/v1/domains/example.com/dkim")
	if code != 200 {
		t.Fatalf("generate: got %d %v", code, out)
	}
	if out["domain"] != "example.com" || out["selector"] != "dkim" ||
		out["record"] != "dkim._domainkey.example.com" || out["enabled"] != true {
		t.Fatalf("generate response: %v", out)
	}
	pub, _ := out["public_key"].(string)
	if !strings.HasPrefix(pub, "v=DKIM1; k=rsa; p=") || len(pub) < 400 {
		t.Fatalf("public_key malformed: %q", pub)
	}

	// The private key is persisted for the rspamd signing vault.
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", "example.com").Error; err != nil {
		t.Fatalf("reload domain: %v", err)
	}
	if !strings.HasPrefix(d.DkimKey, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Fatalf("stored key not PEM: %q", d.DkimKey)
	}

	// Status after generation is stable (same public key derived from storage).
	code, after := dkimDo(t, app, http.MethodGet, "/api/v1/domains/example.com/dkim")
	if code != 200 || after["public_key"] != pub {
		t.Fatalf("status after generate: got %v, want public_key %q", after, pub)
	}

	// Unknown domains answer 404.
	if code, _ := dkimDo(t, app, http.MethodGet, "/api/v1/domains/nope.example.com/dkim"); code != 404 {
		t.Fatalf("status unknown: got %d", code)
	}
	if code, _ := dkimDo(t, app, http.MethodPost, "/api/v1/domains/nope.example.com/dkim"); code != 404 {
		t.Fatalf("generate unknown: got %d", code)
	}
}

func TestDkimPublicKeyTXT(t *testing.T) {
	if got := dkimPublicKeyTXT(""); got != "" {
		t.Fatalf("empty input: got %q", got)
	}
	if got := dkimPublicKeyTXT("not a pem"); got != "" {
		t.Fatalf("garbage input: got %q", got)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	got := dkimPublicKeyTXT(string(privPEM))
	if !strings.HasPrefix(got, "v=DKIM1; k=rsa; p=") || len(got) < 400 {
		t.Fatalf("valid key: got %q", got)
	}
}
