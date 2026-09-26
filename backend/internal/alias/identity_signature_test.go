package alias

import (
	"encoding/json"
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
)

func newIdentitySignatureApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "identities.db")), &gorm.Config{
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
		c.Locals("user", &models.User{Email: "a@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	})
	h.Register(authed)
	return f, db
}

func TestMailIdentitiesCarryResolvedSignatures(t *testing.T) {
	app, db := newIdentitySignatureApp(t)
	seed := []any{
		&models.Alias{Email: "sales@t.example", Localpart: "sales", DomainName: "t.example", Destination: "a@example.com"},
		&models.Alias{Email: "support@t.example", Localpart: "support", DomainName: "t.example", Destination: "a@example.com"},
		&models.Signature{UserEmail: "a@example.com", Name: "Generic", BodyHTML: "<p>generic</p>", BodyText: "generic", DefaultForNew: true},
		&models.Signature{UserEmail: "a@example.com", IdentityEmail: "sales@t.example", Name: "Sales", BodyHTML: "<p>sales</p>", BodyText: "sales", DefaultForNew: true},
	}
	for _, row := range seed {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail/identities", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, raw)
	}
	var ids []MailIdentity
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byEmail := map[string]MailIdentity{}
	for _, idn := range ids {
		byEmail[idn.Email] = idn
	}

	own, ok := byEmail["a@example.com"]
	if !ok {
		t.Fatalf("own identity missing: %+v", ids)
	}
	if own.Signature != "generic" || own.SignatureHTML != "<p>generic</p>" || own.SignatureID == 0 {
		t.Fatalf("own identity should carry the generic default: %+v", own)
	}
	sales, ok := byEmail["sales@t.example"]
	if !ok {
		t.Fatalf("sales alias missing: %+v", ids)
	}
	if sales.Signature != "sales" || sales.SignatureHTML != "<p>sales</p>" {
		t.Fatalf("alias-bound signature not resolved: %+v", sales)
	}
	support, ok := byEmail["support@t.example"]
	if !ok {
		t.Fatalf("support alias missing: %+v", ids)
	}
	if support.Signature != "generic" {
		t.Fatalf("alias fallback missing: %+v", support)
	}
}

func TestMailIdentitiesCarryOwnerSignatureWhenDelegated(t *testing.T) {
	app, db := newIdentitySignatureApp(t)
	owner := models.User{Email: "boss@example.com", Localpart: "boss", DomainName: "example.com", Enabled: true, DisplayedName: "Boss"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := db.Create(&models.MailDelegation{
		OwnerEmail: "boss@example.com", DelegateEmail: "a@example.com", CanSend: true,
	}).Error; err != nil {
		t.Fatalf("seed delegation: %v", err)
	}
	if err := db.Create(&models.Signature{
		UserEmail: "boss@example.com", Name: "Boss", BodyHTML: "<p>boss</p>", BodyText: "boss", DefaultForNew: true,
	}).Error; err != nil {
		t.Fatalf("seed signature: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail/identities", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	defer resp.Body.Close()
	var ids []MailIdentity
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var delegated *MailIdentity
	for i := range ids {
		if ids[i].Email == "boss@example.com" {
			delegated = &ids[i]
		}
	}
	if delegated == nil || !delegated.Delegated {
		t.Fatalf("delegated identity missing: %+v", ids)
	}
	if delegated.Signature != "boss" || delegated.Name != "Boss" {
		t.Fatalf("delegated identity should carry the owner's signature: %+v", delegated)
	}
}
