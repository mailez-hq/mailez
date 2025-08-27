package ldap

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-ldap/ldap/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ldap.db")), &gorm.Config{
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
	return db
}

func seedConfig(t *testing.T, db *gorm.DB, host string, port int, enabled bool) {
	t.Helper()
	cfg := models.LdapConfig{
		ID:         1,
		Enabled:    enabled,
		Host:       host,
		Port:       port,
		Security:   "none",
		BaseDN:     "ou=people,dc=example,dc=org",
		UserFilter: "(objectClass=person)",
		MailAttr:   "email",
		UIDAttr:    "uid",
		NameAttr:   "name",
		DeptAttr:   "department",
		TitleAttr:  "title",
		PhoneAttr:  "telephoneNumber",
		AutoCreate: true,
		SyncMinutes: 60,
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
}

// testUsers builds directory entries that look like a real inetOrgPerson tree.
func testUsers() []*testEntry {
	mk := func(uid, mail, name, dept, phone string) *testEntry {
		return &testEntry{
			dn: "uid=" + uid + ",ou=people,dc=example,dc=org",
			attrs: map[string][]string{
				"objectclass":     {"top", "person", "organizationalPerson", "inetOrgPerson"},
				"uid":             {uid},
				"email":           {mail},
				"password":        {"password"},
				"name":            {name},
				"department":      {dept},
				"telephoneNumber": {phone},
			},
		}
	}
	return []*testEntry{
		mk("alice", "alice@example.com", "Alice Smith", "Engineering", "+86 10 1111"),
		mk("bob", "bob@example.com", "Bob Jones", "Sales", "+86 20 2222"),
	}
}

func TestLDAPAuthenticateAndProvision(t *testing.T) {
	host, port := newTestServer(t, testUsers())
	// Sanity: the test directory answers attribute searches.
	conn, err := ldap.DialURL("ldap://127.0.0.1:" + fmt.Sprint(port))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN: "ou=people,dc=example,dc=org", Scope: ldap.ScopeWholeSubtree,
		Filter: "(&(objectClass=person)(email=alice@example.com))", Attributes: []string{"dn"},
	})
	conn.Close()
	if err != nil {
		t.Fatalf("sanity search: %v", err)
	}
	t.Logf("sanity entries: %d", len(res.Entries))

	db := testDB(t)
	seedConfig(t, db, host, port, true)
	if err := db.Create(&models.Domain{Name: "example.com", MaxQuotaBytes: 2_000_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")

	// Correct directory password authenticates.
	ok, err := svc.Authenticate(context.Background(), "alice@example.com", "password")
	if err != nil || !ok {
		t.Fatalf("authenticate alice: ok=%v err=%v", ok, err)
	}
	// Wrong password rejected.
	if ok, _ := svc.Authenticate(context.Background(), "alice@example.com", "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	// Unknown user is not an error (falls through to local auth).
	if ok, err := svc.Authenticate(context.Background(), "nobody@example.com", "password"); ok || err != nil {
		t.Fatalf("unknown user: ok=%v err=%v", ok, err)
	}

	// Provisioning creates the local account with a random (unusable) local
	// password so the directory stays the credential source.
	if err := svc.EnsureLocalUser(context.Background(), "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	var u models.User
	if err := db.First(&u, "email = ?", "alice@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if !u.Enabled || u.DomainName != "example.com" {
		t.Fatalf("provisioned user: %+v", u)
	}
	// Idempotent.
	if err := svc.EnsureLocalUser(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("second provision: %v", err)
	}
}

func TestLDAPSyncContacts(t *testing.T) {
	host, port := newTestServer(t, testUsers())

	db := testDB(t)
	seedConfig(t, db, host, port, true)
	svc := New(db, "test-secret")

	added, updated, err := svc.SyncContacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || updated != 0 {
		t.Fatalf("first sync: added=%d updated=%d", added, updated)
	}
	var org []models.OrgContact
	if err := db.Order("email").Find(&org).Error; err != nil {
		t.Fatal(err)
	}
	if len(org) != 2 || org[0].Email != "alice@example.com" || org[0].Name != "Alice Smith" || org[0].Department != "Engineering" {
		t.Fatalf("org contacts: %+v", org)
	}
	// Second sync is a no-op (unchanged).
	added, updated, err = svc.SyncContacts(context.Background())
	if err != nil || added != 0 || updated != 0 {
		t.Fatalf("second sync: added=%d updated=%d err=%v", added, updated, err)
	}
}

func TestLDAPDisabled(t *testing.T) {
	db := testDB(t)
	// No config row at all: local auth is untouched.
	svc := New(db, "test-secret")
	if svc.Enabled(context.Background()) {
		t.Fatal("enabled without config")
	}
	if ok, err := svc.Authenticate(context.Background(), "a@example.com", "x"); ok || err != nil {
		t.Fatalf("disabled authenticate: ok=%v err=%v", ok, err)
	}
}
