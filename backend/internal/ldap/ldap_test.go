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
	"mailez/backend/internal/crypto"
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
		ID:          1,
		Enabled:     enabled,
		Host:        host,
		Port:        port,
		Security:    "none",
		BaseDN:      "ou=people,dc=example,dc=org",
		UserFilter:  "(objectClass=person)",
		MailAttr:    "email",
		UIDAttr:     "uid",
		NameAttr:    "name",
		DeptAttr:    "department",
		TitleAttr:   "title",
		PhoneAttr:   "telephoneNumber",
		AutoCreate:  true,
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
	host, port, _ := newTestServer(t, testUsers())
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
	host, port, _ := newTestServer(t, testUsers())

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

func TestLDAPSyncAccountsLifecycle(t *testing.T) {
	host, port, td := newTestServer(t, testUsers())
	db := testDB(t)
	seedConfig(t, db, host, port, true)
	if err := db.Create(&models.Domain{Name: "example.com", MaxQuotaBytes: 2_000_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	// A manually created account must never be touched by the lifecycle sync.
	manual := models.User{
		Email: "manual@example.com", Localpart: "manual", DomainName: "example.com",
		Password: "x", Enabled: true, LdapManaged: false,
	}
	if err := db.Create(&manual).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")

	// First sync provisions both directory users.
	created, disabled, err := svc.SyncAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 2 || disabled != 0 {
		t.Fatalf("first sync: created=%d disabled=%d", created, disabled)
	}
	var u models.User
	if err := db.First(&u, "email = ?", "alice@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if !u.LdapManaged {
		t.Fatal("provisioned account not marked ldap_managed")
	}

	// Bob leaves the directory: his managed account is disabled, alice keeps
	// hers, and the manual account is untouched.
	td.mu.Lock()
	td.entries = td.entries[:1] // drop bob
	td.mu.Unlock()
	created, disabled, err = svc.SyncAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 || disabled != 1 {
		t.Fatalf("after removal: created=%d disabled=%d", created, disabled)
	}
	var bobUser models.User
	if err := db.First(&bobUser, "email = ?", "bob@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if bobUser.Enabled {
		t.Fatal("leaver account still enabled")
	}
	var manualUser models.User
	if err := db.First(&manualUser, "email = ?", "manual@example.com").Error; err != nil || !manualUser.Enabled {
		t.Fatalf("manual account touched: %+v err=%v", manualUser, err)
	}

	// Empty directory: nothing is disabled (config-error protection).
	td.mu.Lock()
	td.entries = nil
	td.mu.Unlock()
	created, disabled, err = svc.SyncAccounts(context.Background())
	if err != nil || created != 0 || disabled != 0 {
		t.Fatalf("empty directory: created=%d disabled=%d err=%v", created, disabled, err)
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

func TestMatchEntryFilter(t *testing.T) {
	u := testUsers()[0]
	if !matchEntryFilter("(objectClass=person)", u) {
		t.Fatal("person filter should match alice")
	}
	if !matchEntryFilter("(&(objectClass=person)(email=alice@example.com))", u) {
		t.Fatal("combined filter should match alice")
	}
}

func TestServerPersonSearch(t *testing.T) {
	host, port, td := newTestServer(t, testUsers())
	_ = td
	conn, err := ldap.DialURL(fmt.Sprintf("ldap://%s:%d", host, port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN: "ou=people,dc=example,dc=org", Scope: ldap.ScopeWholeSubtree,
		Filter: "(objectClass=person)", Attributes: []string{"dn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("expected 2 person entries, got %d", len(res.Entries))
	}
}

// adTestUsers mimics an Active Directory where the mail attribute is empty
// and addresses come from userPrincipalName (or uid@domain).
func adTestUsers() []*testEntry {
	mk := func(uid, upn string) *testEntry {
		attrs := map[string][]string{
			"objectclass": {"top", "person", "organizationalPerson", "inetOrgPerson"},
			"uid":         {uid},
			"password":    {"password"},
			"name":        {uid + " User"},
		}
		if upn != "" {
			attrs["userPrincipalName"] = []string{upn}
		}
		return &testEntry{dn: "uid=" + uid + ",ou=people,dc=example,dc=org", attrs: attrs}
	}
	return []*testEntry{
		mk("alice", "alice@example.com"), // UPN carries the mailbox
		mk("carol", ""),                  // sAMAccountName-only: uid@domain
	}
}

func TestADMappingAuth(t *testing.T) {
	host, port, _ := newTestServer(t, adTestUsers())
	db := testDB(t)
	cfg := models.LdapConfig{
		ID: 1, Enabled: true, Host: host, Port: port, Security: "none",
		BaseDN: "ou=people,dc=example,dc=org", UserFilter: "(objectClass=person)",
		MailAttr: "mail", UIDAttr: "uid", UpnAttr: "userPrincipalName",
		EmailDomain: "example.com", NameAttr: "name", AutoCreate: true, SyncMinutes: 60,
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Domain{Name: "example.com", MaxQuotaBytes: 1_000_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")

	// UPN-based login.
	ok, err := svc.Authenticate(context.Background(), "alice@example.com", "password")
	if err != nil || !ok {
		t.Fatalf("UPN auth: ok=%v err=%v", ok, err)
	}
	// sAMAccountName-only login (uid@EmailDomain).
	ok, err = svc.Authenticate(context.Background(), "carol@example.com", "password")
	if err != nil || !ok {
		t.Fatalf("uid-derived auth: ok=%v err=%v", ok, err)
	}
	// Provisioning derives the mailbox from UPN / uid@domain.
	if err := svc.EnsureLocalUser(context.Background(), "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureLocalUser(context.Background(), "carol@example.com"); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := db.Model(&models.User{}).Where("email IN ?", []string{"alice@example.com", "carol@example.com"}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("provisioned users: %d", n)
	}
}

func TestSyncGroups(t *testing.T) {
	users := testUsers()
	users = append(users, &testEntry{
		dn: "cn=eng-team,ou=people,dc=example,dc=org",
		attrs: map[string][]string{
			"objectclass": {"top", "groupOfNames"},
			"cn":          {"eng-team"},
			"member": {
				"uid=alice,ou=people,dc=example,dc=org",
				"uid=bob,ou=people,dc=example,dc=org",
			},
		},
	})
	host, port, _ := newTestServer(t, users)
	db := testDB(t)
	cfg := models.LdapConfig{
		ID: 1, Enabled: true, Host: host, Port: port, Security: "none",
		BaseDN: "ou=people,dc=example,dc=org", UserFilter: "(objectClass=person)",
		MailAttr: "email", UIDAttr: "uid", UpnAttr: "userPrincipalName",
		EmailDomain: "example.com", NameAttr: "name", AutoCreate: true, SyncMinutes: 60,
		SyncGroups: true, GroupFilter: "(objectClass=groupOfNames)",
		GroupNameAttr: "cn", GroupMailAttr: "mail", GroupMemberAttr: "member",
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Domain{Name: "example.com", MaxQuotaBytes: 1_000_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	// A manual alias with the same address must win over the synced group.
	if err := db.Create(&models.Alias{
		Email: "eng-team@example.com", Localpart: "eng-team", DomainName: "example.com",
		Destination: "manual@example.com",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")
	if _, _, err := svc.SyncAccounts(context.Background()); err != nil {
		t.Fatal(err)
	}

	created, updated, disabled, err := svc.SyncGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 || updated != 0 || disabled != 0 {
		t.Fatalf("group sync over manual alias: created=%d updated=%d disabled=%d", created, updated, disabled)
	}
	var alias models.Alias
	if err := db.First(&alias, "email = ?", "eng-team@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if alias.LdapGroup || alias.Destination != "manual@example.com" {
		t.Fatalf("manual alias overwritten: %+v", alias)
	}

	// Remove the manual alias; the group now creates its own.
	if err := db.Delete(&models.Alias{}, "email = ?", "eng-team@example.com").Error; err != nil {
		t.Fatal(err)
	}
	created, updated, disabled, err = svc.SyncGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("group alias created: %d", created)
	}
	if err := db.First(&alias, "email = ?", "eng-team@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if !alias.LdapGroup || alias.Destination != "alice@example.com,bob@example.com" {
		t.Fatalf("group alias: %+v", alias)
	}

	// Group vanishes -> the synced alias is disabled, not deleted.
	host2, port2, _ := newTestServer(t, testUsers())
	if err := db.Model(&models.LdapConfig{}).Where("id = ?", 1).Updates(map[string]any{"host": host2, "port": port2}).Error; err != nil {
		t.Fatal(err)
	}
	created, updated, disabled, err = svc.SyncGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 || updated != 0 || disabled != 1 {
		t.Fatalf("group removal: created=%d updated=%d disabled=%d", created, updated, disabled)
	}
	if err := db.First(&alias, "email = ?", "eng-team@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if !alias.Disabled {
		t.Fatal("vanished group alias still enabled")
	}
}

// TestOpenLDAPGroupsReal exercises SyncGroups against the local OpenLDAP
// container (skipped when it is not running).
func TestOpenLDAPGroupsReal(t *testing.T) {
	if _, err := ldap.DialURL("ldap://127.0.0.1:1389"); err != nil {
		t.Skip("local OpenLDAP container not running")
	}
	db := testDB(t)
	cfg := models.LdapConfig{
		ID: 1, Enabled: true, Host: "127.0.0.1", Port: 1389, Security: "none",
		BaseDN: "ou=people,dc=example,dc=com", UserFilter: "(objectClass=person)",
		BindDN:   "cn=admin,dc=example,dc=com",
		MailAttr: "mail", UIDAttr: "uid", UpnAttr: "userPrincipalName",
		EmailDomain: "example.com", NameAttr: "cn",
		DeptAttr: "departmentNumber", TitleAttr: "title", PhoneAttr: "telephoneNumber",
		AutoCreate: true, SyncMinutes: 60,
		SyncGroups: true, GroupFilter: "(objectClass=groupOfNames)",
		GroupNameAttr: "cn", GroupMailAttr: "mail", GroupMemberAttr: "member",
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	enc, err := crypto.Encrypt("test-secret", "adminpw")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.LdapConfig{}).Where("id = ?", 1).Update("bind_password_enc", enc).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Domain{Name: "example.com", MaxQuotaBytes: 1_000_000_000}).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")
	cCreated, _, cerr := svc.SyncAccounts(context.Background())
	t.Logf("real sync accounts created=%d err=%v", cCreated, cerr)
	// Simulate the E2E flow: the account sync provisions bob first.
	if err := svc.EnsureLocalUser(context.Background(), "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	created, updated, disabled, err := svc.SyncGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real sync groups: created=%d updated=%d disabled=%d", created, updated, disabled)
	var alias models.Alias
	if err := db.First(&alias, "email = ?", "eng@example.com").Error; err != nil {
		t.Fatalf("alias missing: %v", err)
	}
	t.Logf("alias: %+v", alias)
}

// TestOpenLDAPLiveResolve exercises delivery-time expansion against the real
// OpenLDAP container with a nested group.
func TestOpenLDAPLiveResolve(t *testing.T) {
	if _, err := ldap.DialURL("ldap://127.0.0.1:1389"); err != nil {
		t.Skip("local OpenLDAP container not running")
	}
	db := testDB(t)
	cfg := models.LdapConfig{
		ID: 1, Enabled: true, Host: "127.0.0.1", Port: 1389, Security: "none",
		BaseDN: "ou=people,dc=example,dc=com", UserFilter: "(objectClass=person)",
		BindDN: "cn=admin,dc=example,dc=com",
		MailAttr: "mail", UIDAttr: "uid", UpnAttr: "userPrincipalName",
		EmailDomain: "example.com", NameAttr: "cn", AutoCreate: true, SyncMinutes: 60,
		SyncGroups: true, GroupFilter: "(objectClass=groupOfNames)",
		GroupNameAttr: "cn", GroupMailAttr: "mail", GroupMemberAttr: "member",
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	enc, err := crypto.Encrypt("test-secret", "adminpw")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.LdapConfig{}).Where("id = ?", 1).Update("bind_password_enc", enc).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")
	members, err := svc.ResolveGroupMembers(context.Background(), "all@example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("live resolve all: %v", members)
	if len(members) < 2 || members[0] != "alice@example.com" || members[1] != "bob@example.com" {
		t.Fatalf("expected alice+bob, got %v", members)
	}
}

func TestResolveGroupMembersNested(t *testing.T) {
	users := testUsers()
	users = append(users,
		&testEntry{
			dn: "cn=eng,ou=people,dc=example,dc=org",
			attrs: map[string][]string{
				"objectclass": {"top", "groupOfNames"},
				"cn":          {"eng"},
				"member": {
					"uid=alice,ou=people,dc=example,dc=org",
					"uid=bob,ou=people,dc=example,dc=org",
				},
			},
		},
		&testEntry{
			dn: "cn=all,ou=people,dc=example,dc=org",
			attrs: map[string][]string{
				"objectclass": {"top", "groupOfNames"},
				"cn":          {"all"},
				"member": {
					"cn=eng,ou=people,dc=example,dc=org",
					"uid=alice,ou=people,dc=example,dc=org",
				},
			},
		},
	)
	host, port, _ := newTestServer(t, users)
	db := testDB(t)
	cfg := models.LdapConfig{
		ID: 1, Enabled: true, Host: host, Port: port, Security: "none",
		BaseDN: "ou=people,dc=example,dc=org", UserFilter: "(objectClass=person)",
		MailAttr: "email", UIDAttr: "uid", UpnAttr: "userPrincipalName",
		EmailDomain: "example.com", NameAttr: "name", AutoCreate: true, SyncMinutes: 60,
		SyncGroups: true, GroupFilter: "(objectClass=groupOfNames)",
		GroupNameAttr: "cn", GroupMailAttr: "mail", GroupMemberAttr: "member",
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")

	// Nested group expansion with dedup: all -> eng -> alice+bob, plus alice.
	members, err := svc.ResolveGroupMembers(context.Background(), "all@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0] != "alice@example.com" || members[1] != "bob@example.com" {
		t.Fatalf("nested members: %v", members)
	}

	// Cache serves the second call with the same result.
	members2, err := svc.ResolveGroupMembers(context.Background(), "all@example.com")
	if err != nil || len(members2) != 2 {
		t.Fatalf("cached members: %v err=%v", members2, err)
	}
}

func TestResolveGroupMembersCycle(t *testing.T) {
	users := testUsers()
	users = append(users,
		&testEntry{
			dn: "cn=a,ou=people,dc=example,dc=org",
			attrs: map[string][]string{
				"objectclass": {"top", "groupOfNames"},
				"cn":          {"a"},
				"member": {
					"cn=b,ou=people,dc=example,dc=org",
					"uid=alice,ou=people,dc=example,dc=org",
				},
			},
		},
		&testEntry{
			dn: "cn=b,ou=people,dc=example,dc=org",
			attrs: map[string][]string{
				"objectclass": {"top", "groupOfNames"},
				"cn":          {"b"},
				"member": {
					"cn=a,ou=people,dc=example,dc=org",
					"uid=bob,ou=people,dc=example,dc=org",
				},
			},
		},
	)
	host, port, _ := newTestServer(t, users)
	db := testDB(t)
	cfg := models.LdapConfig{
		ID: 1, Enabled: true, Host: host, Port: port, Security: "none",
		BaseDN: "ou=people,dc=example,dc=org", UserFilter: "(objectClass=person)",
		MailAttr: "email", UIDAttr: "uid", UpnAttr: "userPrincipalName",
		EmailDomain: "example.com", NameAttr: "name", AutoCreate: true, SyncMinutes: 60,
		SyncGroups: true, GroupFilter: "(objectClass=groupOfNames)",
		GroupNameAttr: "cn", GroupMailAttr: "mail", GroupMemberAttr: "member",
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db, "test-secret")
	members, err := svc.ResolveGroupMembers(context.Background(), "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// a -> b -> a (visited, skipped) + bob; a -> alice.
	if len(members) != 2 || members[0] != "alice@example.com" || members[1] != "bob@example.com" {
		t.Fatalf("cycle members: %v", members)
	}
}
