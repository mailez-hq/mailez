package license

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type userRow struct {
	ID    uint
	Email string
}

func (userRow) TableName() string { return "users" }

func testDB(t *testing.T, emails ...string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&userRow{}); err != nil {
		t.Fatal(err)
	}
	for _, e := range emails {
		if err := db.Create(&userRow{Email: e}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func enterprise(mailboxes int, expires string) License {
	lic := License{
		Version:      1,
		Edition:      EditionEnterprise,
		Licensee:     "Acme",
		MaxMailboxes: mailboxes,
		IssuedAt:     time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		ExpiresAt:    expires,
	}
	if expires == "" {
		lic.ExpiresAt = time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return lic
}

func mustEnvelope(t *testing.T, lic License) string {
	t.Helper()
	env, err := Sign(lic, DevPrivateKey())
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func TestSignParseRoundtrip(t *testing.T) {
	env := mustEnvelope(t, enterprise(500, ""))
	lic, err := Parse(env)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if lic.Edition != EditionEnterprise || lic.MaxMailboxes != 500 || lic.Licensee != "Acme" {
		t.Fatalf("unexpected license: %+v", lic)
	}
}

func TestTamperedPayloadRejected(t *testing.T) {
	env := mustEnvelope(t, enterprise(500, ""))
	payload := strings.SplitN(env, ".", 2)[0]
	tampered := payload + "A" // flip the payload bytes
	parts := strings.SplitN(env, ".", 2)
	if _, err := Parse(parts[0] + "." + tampered); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if _, err := Parse(tampered + "." + parts[1]); err == nil {
		t.Fatal("tampered payload accepted")
	}
}

func TestServiceExpiryDoesNotBlockUsage(t *testing.T) {
	env := mustEnvelope(t, enterprise(100, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)))
	m, err := Load("", env, true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !m.ServiceExpired(time.Now()) {
		t.Fatal("service should be reported expired")
	}
	// Perpetual usage right: the mailbox cap still applies and provisioning
	// inside the cap must be allowed even with an expired service.
	if err := m.CheckCapacity(testDB(t)); err != nil {
		t.Fatalf("service-expired license must not block capacity: %v", err)
	}
	st := m.Status(testDB(t))
	if st.Valid != true || st.ServiceValid != false {
		t.Fatalf("expected valid usage + expired service, got %+v", st)
	}
}

func TestCapacity(t *testing.T) {
	cases := []struct {
		name      string
		lic       License
		users     []string
		wantAllow bool
	}{
		{"under limit", enterprise(5, ""), []string{"a@x", "b@x"}, true},
		{"at limit", enterprise(2, ""), []string{"a@x", "b@x"}, false},
		{"over limit", enterprise(2, ""), []string{"a@x", "b@x", "c@x"}, false},
		{"unlimited", enterprise(0, ""), []string{"a@x", "b@x", "c@x"}, true},
		{"dev edition", DevLicense(), []string{"a@x", "b@x", "c@x"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var m *Manager
			if c.lic.Edition == EditionDev {
				m = &Manager{lic: DevLicense()}
			} else {
				env := mustEnvelope(t, c.lic)
				var err error
				m, err = Load("", env, true)
				if err != nil {
					t.Fatalf("load: %v", err)
				}
			}
			err := m.CheckCapacity(testDB(t, c.users...))
			if c.wantAllow && err != nil {
				t.Fatalf("expected allow, got %v", err)
			}
			if !c.wantAllow && err == nil {
				t.Fatal("expected deny, got allow")
			}
		})
	}
}

func TestLoadRequired(t *testing.T) {
	if _, err := Load("", "", true); err == nil {
		t.Fatal("missing license with required=true accepted")
	}
	m, err := Load("", "", false)
	if err != nil {
		t.Fatalf("default load: %v", err)
	}
	if m.Edition() != EditionDev {
		t.Fatalf("expected dev edition, got %q", m.Edition())
	}
}

func TestStatus(t *testing.T) {
	env := mustEnvelope(t, enterprise(3, ""))
	m, err := Load("", env, true)
	if err != nil {
		t.Fatal(err)
	}
	st := m.Status(testDB(t, "a@x", "b@x"))
	if st.MaxMailboxes != 3 || st.Used != 2 || !st.Valid || !st.ServiceValid || !st.Required {
		t.Fatalf("unexpected status: %+v", st)
	}
}
