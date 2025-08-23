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

func newIdentitiesTestHandler(t *testing.T, user *models.User) (*Handler, *fiber.App) {
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

func TestMailIdentities(t *testing.T) {
	user := &models.User{
		Email:         "amy@example.com",
		Localpart:     "amy",
		DomainName:    "example.com",
		DisplayedName: "Amy",
	}
	h, app := newIdentitiesTestHandler(t, user)
	if err := h.DB.Create(&models.Domain{Name: "example.com", DkimKey: "PEM"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	if err := h.DB.Create(&models.Alias{
		Email: "team@example.com", Localpart: "team", DomainName: "example.com",
		Destination: "amy@example.com",
	}).Error; err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	// A disabled alias must not be offered as an identity.
	if err := h.DB.Create(&models.Alias{
		Email: "old@example.com", Localpart: "old", DomainName: "example.com",
		Destination: "amy@example.com", Disabled: true,
	}).Error; err != nil {
		t.Fatalf("seed disabled alias: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail/identities", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d body %s", resp.StatusCode, body)
	}
	var ids []MailIdentity
	if err := json.Unmarshal(body, &ids); err != nil {
		t.Fatalf("unmarshal %q: %v", body, err)
	}
	if len(ids) != 2 {
		t.Fatalf("identities: got %d, want 2 (%s)", len(ids), body)
	}
	if ids[0].Email != "amy@example.com" || ids[0].Name != "Amy" || !ids[0].DkimEnabled {
		t.Fatalf("primary identity: %+v", ids[0])
	}
	if ids[1].Email != "team@example.com" || !ids[1].DkimEnabled {
		t.Fatalf("alias identity: %+v", ids[1])
	}
}

func TestUserMaySendAs(t *testing.T) {
	user := &models.User{
		Email:      "amy@example.com",
		Localpart:  "amy",
		DomainName: "example.com",
	}
	h, _ := newIdentitiesTestHandler(t, user)
	if err := h.DB.Create(&models.Domain{Name: "example.com"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	if err := h.DB.Create(&models.Alias{
		Email: "team@example.com", Localpart: "team", DomainName: "example.com",
		Destination: "amy@example.com",
	}).Error; err != nil {
		t.Fatalf("seed alias: %v", err)
	}

	for _, tc := range []struct {
		from string
		want bool
	}{
		{"amy@example.com", true},
		{"AMY@example.com", true},
		{"team@example.com", true},
		{"stranger@example.com", false},
		{"", true},
	} {
		if got := MaySendAs(h.App, user, tc.from); got != tc.want {
			t.Errorf("userMaySendAs(%q) = %v, want %v", tc.from, got, tc.want)
		}
	}
}

func TestMaySendAsRequiresExactDestination(t *testing.T) {
	// user "a@example.com" must not match a destination "anna@example.com":
	// substring matching would let them spoof that alias.
	user := &models.User{
		Email:      "a@example.com",
		Localpart:  "a",
		DomainName: "example.com",
	}
	h, _ := newIdentitiesTestHandler(t, user)
	if err := h.DB.Create(&models.Domain{Name: "example.com"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	if err := h.DB.Create(&models.Alias{
		Email: "team@example.com", Localpart: "team", DomainName: "example.com",
		Destination: "anna@example.com",
	}).Error; err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	if MaySendAs(h.App, user, "team@example.com") {
		t.Error("substring destination match must not grant send-as")
	}
	// An exact destination entry in a multi-address CSV still grants it.
	if err := h.DB.Model(&models.Alias{}).
		Where("email = ?", "team@example.com").
		Update("destination", "other@example.com, a@example.com").Error; err != nil {
		t.Fatalf("update alias: %v", err)
	}
	if !MaySendAs(h.App, user, "team@example.com") {
		t.Error("exact destination entry must grant send-as")
	}
}
