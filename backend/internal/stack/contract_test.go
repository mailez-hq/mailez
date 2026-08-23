package stack

// Contract tests for the /stack API consumed by the mail stack (postfix /
// dovecot / nginx). The response formats below must stay byte-compatible with
// the internal API contract, so these tests pin the exact wire contract
// (status codes, JSON quoting, list formatting).

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/password"
)

const testDkimKey = "-----BEGIN RSA PRIVATE KEY-----\nMOCKKEY\n-----END RSA PRIVATE KEY-----"

var testUserHash = sync.OnceValue(func() string {
	h, err := password.Hash("secret123")
	if err != nil {
		panic(err)
	}
	return h
})

// newContractHarness builds an isolated Handler + Fiber app on a temp DB.
func newContractHarness(t *testing.T) (*Handler, *fiber.App) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "contract.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	cfg := core.Config{
		SecretKey:          "contract-test-secret",
		RecipientDelimiter: "+",
		Subnet:             "192.168.206.0/24",
		Hostname:           "mail.example.com",
		Domain:             "example.com",
		MessageRateLimit:   200,
		DkimSelector:       "dkim",
		MailKeeperAddress:  "127.0.0.1",
		MtaAddress:         "127.0.0.1",
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	h := New(db, mgr, cfg, nil)
	app := fiber.New()
	h.Register(app.Group("/stack"))
	return h, app
}

func seedContractData(t *testing.T, h *Handler) {
	t.Helper()
	db := h.DB
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed %T: %v", v, err)
		}
	}
	create(&models.Domain{Name: "example.com", MaxQuotaBytes: 10 << 30, DkimKey: testDkimKey})
	create(&models.Domain{Name: "spoof.example.com"})
	create(&models.Alternative{Name: "alt.example.com", DomainName: "example.com"})
	create(&models.Relay{Name: "relay.example.com", SMTP: "relay.example.com:2525"})
	create(&models.Relay{Name: "mxrelay.example.com", SMTP: "mx:mxrelay.example.com"})
	create(&models.Relay{Name: "lmtprelay.example.com", SMTP: "lmtp:[imap.example.com]:2525"})
	create(&models.User{
		Email: "alice@example.com", Localpart: "alice", DomainName: "example.com",
		Password: testUserHash(), Enabled: true, QuotaBytes: 1_000_000_000, GlobalAdmin: true,
	})
	create(&models.User{
		Email: "bob@example.com", Localpart: "bob", DomainName: "example.com",
		Password: testUserHash(), Enabled: true, QuotaBytes: 2_000_000_000, AllowSpoofing: true,
	})
	create(&models.User{
		Email: "carol@spoof.example.com", Localpart: "carol", DomainName: "spoof.example.com",
		Password: testUserHash(), Enabled: true, AllowSpoofing: true,
	})
	create(&models.Alias{
		Email: "team@example.com", Localpart: "team", DomainName: "example.com",
		Destination: "alice@example.com,bob@example.com",
	})
	create(&models.Alias{
		Email: "*@example.com", Localpart: "*", DomainName: "example.com",
		Wildcard: true, Destination: "alice@example.com",
	})
	enc, err := crypto.Encrypt(h.Cfg.SecretKey, "fetchpass")
	if err != nil {
		t.Fatalf("encrypt fetch password: %v", err)
	}
	create(&models.Fetch{
		UserEmail: "alice@example.com", Protocol: "imap", Host: "imap.example.org",
		Port: 993, TLS: true, Keep: true, Username: "alice", Password: enc, Folders: "INBOX",
	})
}

func doGet(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func doPost(t *testing.T, app *fiber.App, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func doAuthReq(t *testing.T, app *fiber.App, path string, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(body)
}

func TestAuthContract(t *testing.T) {
	h, app := newContractHarness(t)
	db := h.DB
	for _, v := range []any{
		&models.Domain{Name: "example.com"},
		&models.User{
			Email: "alice@example.com", Localpart: "alice", DomainName: "example.com",
			Password: testUserHash(), Enabled: true, GlobalAdmin: true, EnableImap: true, EnablePop: true,
		},
		&models.User{
			Email: "bob@example.com", Localpart: "bob", DomainName: "example.com",
			Password: testUserHash(), Enabled: true, EnableImap: true, EnablePop: false,
		},
		&models.User{
			Email: "deactivated@example.com", Localpart: "deactivated", DomainName: "example.com",
			Password: testUserHash(), Enabled: false,
		},
	} {
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed %T: %v", v, err)
		}
	}
	// GORM omits zero-value booleans with a `default` tag from INSERT, so the
	// DB default (true) wins; write the false flags explicitly afterwards.
	for email, field := range map[string]string{
		"deactivated@example.com": "enabled",
		"bob@example.com":         "enable_pop",
	} {
		if err := db.Model(&models.User{}).Where("email = ?", email).Update(field, false).Error; err != nil {
			t.Fatalf("clear %s for %s: %v", field, email, err)
		}
	}
	ctx := context.Background()

	// --- /internal/auth/user (SSO gate for webmail) ---
	if code, _, _ := doAuthReq(t, app, "/stack/auth/user", nil); code != 403 {
		t.Fatalf("auth/user no session: got %d", code)
	}
	sid, err := h.Auth.CreateSession(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	code, headers, _ := doAuthReq(t, app, "/stack/auth/user", map[string]string{
		"Cookie": h.Auth.SessionName + "=" + sid,
	})
	if code != 200 || headers.Get("X-User") != "alice@example.com" {
		t.Fatalf("auth/user session: got %d X-User=%q", code, headers.Get("X-User"))
	}
	if tok := headers.Get("X-User-Token"); !strings.HasPrefix(tok, "token-") {
		t.Fatalf("auth/user token: got %q", tok)
	}
	deadSid, err := h.Auth.CreateSession(ctx, "deactivated@example.com")
	if err != nil {
		t.Fatalf("create disabled session: %v", err)
	}
	if code, _, _ := doAuthReq(t, app, "/stack/auth/user", map[string]string{
		"Cookie": h.Auth.SessionName + "=" + deadSid,
	}); code != 403 {
		t.Fatalf("auth/user disabled: got %d", code)
	}

	// --- /internal/auth/admin ---
	if code, _, _ := doAuthReq(t, app, "/stack/auth/admin", map[string]string{
		"Cookie": h.Auth.SessionName + "=" + sid,
	}); code != 200 {
		t.Fatalf("auth/admin admin session: got %d", code)
	}
	bobSid, err := h.Auth.CreateSession(ctx, "bob@example.com")
	if err != nil {
		t.Fatalf("create bob session: %v", err)
	}
	if code, _, _ := doAuthReq(t, app, "/stack/auth/admin", map[string]string{
		"Cookie": h.Auth.SessionName + "=" + bobSid,
	}); code != 403 {
		t.Fatalf("auth/admin non-admin: got %d", code)
	}

	// --- /internal/auth/basic (webdav) ---
	basic := func(user, pw string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pw))
	}
	if code, headers, _ := doAuthReq(t, app, "/stack/auth/basic", map[string]string{
		"Authorization": basic("alice@example.com", "secret123"),
	}); code != 200 || headers.Get("X-User") != "alice@example.com" {
		t.Fatalf("auth/basic ok: got %d X-User=%q", code, headers.Get("X-User"))
	}
	if code, _, _ := doAuthReq(t, app, "/stack/auth/basic", map[string]string{
		"Authorization": basic("alice@example.com", "wrong"),
	}); code != 401 {
		t.Fatalf("auth/basic wrong pw: got %d", code)
	}
	if code, headers, _ := doAuthReq(t, app, "/stack/auth/basic", nil); code != 401 || headers.Get("WWW-Authenticate") == "" {
		t.Fatalf("auth/basic missing header: got %d", code)
	}
	if code, _, _ := doAuthReq(t, app, "/stack/auth/basic", map[string]string{
		"Authorization": basic("deactivated@example.com", "secret123"),
	}); code != 401 {
		t.Fatalf("auth/basic disabled: got %d", code)
	}

	// --- /internal/auth/email (nginx mail proxy auth) ---
	mailHeaders := func(m map[string]string) map[string]string {
		out := map[string]string{"Auth-Method": "plain", "Auth-Protocol": "smtp"}
		for k, v := range m {
			out[k] = v
		}
		return out
	}

	// Incoming (unauthenticated) SMTP is routed, not rejected.
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", map[string]string{
		"Auth-Method": "none", "Auth-Protocol": "smtp",
	})
	if code != 200 || headers.Get("Auth-Status") != "OK" || headers.Get("Auth-Port") != "25" {
		t.Fatalf("auth/email inbound: got %d %q %q", code, headers.Get("Auth-Status"), headers.Get("Auth-Port"))
	}

	// Plain submission with the mailbox password.
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", mailHeaders(map[string]string{
		"Auth-User": "alice@example.com", "Auth-Pass": "secret123",
	}))
	if code != 200 || headers.Get("Auth-Status") != "OK" ||
		headers.Get("Auth-Server") != "127.0.0.1" || headers.Get("Auth-Port") != "1587" {
		t.Fatalf("auth/email ok: got %d %q server=%q port=%q", code,
			headers.Get("Auth-Status"), headers.Get("Auth-Server"), headers.Get("Auth-Port"))
	}

	// Bad password keeps the 200 envelope but reports the SMTP error code.
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", mailHeaders(map[string]string{
		"Auth-User": "alice@example.com", "Auth-Pass": "wrong",
	}))
	if code != 200 || headers.Get("Auth-Status") != "Authentication credentials invalid" ||
		headers.Get("Auth-Error-Code") != "535 5.7.8" ||
		headers.Get("Auth-User-Exists") != "True" || headers.Get("Auth-Wait") != "0" {
		t.Fatalf("auth/email bad pw: got %d %q err=%q exists=%q", code,
			headers.Get("Auth-Status"), headers.Get("Auth-Error-Code"), headers.Get("Auth-User-Exists"))
	}

	// Unknown users are flagged for postfix.
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", mailHeaders(map[string]string{
		"Auth-User": "nobody@example.com", "Auth-Pass": "secret123",
	}))
	if code != 200 || headers.Get("Auth-User-Exists") != "False" {
		t.Fatalf("auth/email unknown: got %d exists=%q", code, headers.Get("Auth-User-Exists"))
	}

	// Disabled accounts are rejected even with the right password.
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", mailHeaders(map[string]string{
		"Auth-User": "deactivated@example.com", "Auth-Pass": "secret123",
	}))
	if code != 200 || headers.Get("Auth-Status") != "Authentication credentials invalid" {
		t.Fatalf("auth/email disabled: got %d %q", code, headers.Get("Auth-Status"))
	}

	// Temporary webmail tokens only authenticate on the webmail ports.
	tok, err := h.Auth.CreateTempToken(ctx, "alice@example.com", sid)
	if err != nil {
		t.Fatalf("create temp token: %v", err)
	}
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", map[string]string{
		"Auth-Method": "plain", "Auth-Protocol": "imap", "Auth-Port": "1143",
		"Auth-User": "alice@example.com", "Auth-Pass": tok,
	})
	if code != 200 || headers.Get("Auth-Status") != "OK" {
		t.Fatalf("auth/email temp token: got %d %q", code, headers.Get("Auth-Status"))
	}

	// App tokens (32-hex application passwords) authenticate anywhere.
	appToken := "0123456789abcdef0123456789abcdef"
	appHash, err := password.HashPBKDF2SHA256(appToken)
	if err != nil {
		t.Fatalf("hash app token: %v", err)
	}
	if err := db.Create(&models.Token{UserEmail: "alice@example.com", Password: appHash}).Error; err != nil {
		t.Fatalf("seed app token: %v", err)
	}
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", mailHeaders(map[string]string{
		"Auth-User": "alice@example.com", "Auth-Pass": appToken,
	}))
	if code != 200 || headers.Get("Auth-Status") != "OK" {
		t.Fatalf("auth/email app token: got %d %q", code, headers.Get("Auth-Status"))
	}

	// POP3 is denied when the user has it disabled.
	code, headers, _ = doAuthReq(t, app, "/stack/auth/email", map[string]string{
		"Auth-Method": "plain", "Auth-Protocol": "pop3",
		"Auth-User": "bob@example.com", "Auth-Pass": "secret123",
	})
	if code != 200 || headers.Get("Auth-Status") == "OK" {
		t.Fatalf("auth/email pop3 disabled: got %d %q", code, headers.Get("Auth-Status"))
	}

	// Unsupported auth methods are a hard failure.
	if code, _, _ := doAuthReq(t, app, "/stack/auth/email", map[string]string{
		"Auth-Method": "garbage", "Auth-Protocol": "imap",
		"Auth-User": "alice@example.com", "Auth-Pass": "secret123",
	}); code != 500 {
		t.Fatalf("auth/email bad method: got %d", code)
	}
}

func TestPostfixDomainContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	if code, body := doGet(t, app, "/stack/postfix/domain/example.com"); code != 200 || body != `"example.com"` {
		t.Fatalf("domain: got %d %q", code, body)
	}
	if code, body := doGet(t, app, "/stack/postfix/domain/alt.example.com"); code != 200 || body != `"example.com"` {
		t.Fatalf("alternative: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/postfix/domain/%5B1.2.3.4%5D"); code != 404 {
		t.Fatalf("bracketed domain: got %d", code)
	}
	if code, _ := doGet(t, app, "/stack/postfix/domain/unknown.example.com"); code != 404 {
		t.Fatalf("unknown domain: got %d", code)
	}
}

func TestPostfixMailboxContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	if code, body := doGet(t, app, "/stack/postfix/mailbox/alice@example.com"); code != 200 || body != `"alice@example.com"` {
		t.Fatalf("mailbox: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/postfix/mailbox/nobody@example.com"); code != 404 {
		t.Fatalf("missing mailbox: got %d", code)
	}
}

func TestPostfixAliasContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	cases := []struct {
		path string
		want string
	}{
		{"/stack/postfix/alias/team@example.com", `"alice@example.com,bob@example.com"`},
		{"/stack/postfix/alias/team+detail@example.com", `"alice+detail@example.com,bob+detail@example.com"`},
		{"/stack/postfix/alias/team%2Bdetail@example.com", `"alice+detail@example.com,bob+detail@example.com"`},
		{"/stack/postfix/alias/alice@example.com", `"alice@example.com"`},
		{"/stack/postfix/alias/example.com", `"example.com"`},
		{"/stack/postfix/alias/wildcard@example.com", `"alice@example.com"`},
	}
	for _, c := range cases {
		if code, body := doGet(t, app, c.path); code != 200 || body != c.want {
			t.Errorf("%s: got %d %q, want %q", c.path, code, body, c.want)
		}
	}

	for _, path := range []string{
		"/stack/postfix/alias/nobody@unknown.example.com",
		"/stack/postfix/alias/%22quoted%22@example.com",
		"/stack/postfix/alias/a%40b@example.com",
	} {
		if code, _ := doGet(t, app, path); code != 404 {
			t.Errorf("%s: got %d, want 404", path, code)
		}
	}
}

func TestPostfixTransportContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	cases := []struct {
		path string
		want string
	}{
		{"/stack/postfix/transport/alice@relay.example.com", `"smtp:[relay.example.com]:2525"`},
		{"/stack/postfix/transport/alice@mxrelay.example.com", `"smtp:mxrelay.example.com"`},
		{"/stack/postfix/transport/alice@lmtprelay.example.com", `"lmtp:imap.example.com:2525"`},
	}
	for _, c := range cases {
		if code, body := doGet(t, app, c.path); code != 200 || body != c.want {
			t.Errorf("%s: got %d %q, want %q", c.path, code, body, c.want)
		}
	}

	for _, path := range []string{
		"/stack/postfix/transport/alice@unknown.example.com",
		"/stack/postfix/transport/*",
	} {
		if code, _ := doGet(t, app, path); code != 404 {
			t.Errorf("%s: got %d, want 404", path, code)
		}
	}
}

func TestPostfixSenderLoginContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	code, body := doGet(t, app, "/stack/postfix/sender/login/alice@example.com")
	if code != 200 {
		t.Fatalf("sender login: got %d", code)
	}
	got := sortedCSV(t, body)
	want := []string{"alice@example.com", "bob@example.com"} // bob may spoof within the domain
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sender login: got %v, want %v", got, want)
	}

	// A localpart nobody owns still lists the spoofers of the domain
	// (isolated on a domain without wildcard aliases).
	code, body = doGet(t, app, "/stack/postfix/sender/login/anyone@spoof.example.com")
	if code != 200 {
		t.Fatalf("sender login (spoof): got %d", code)
	}
	got = sortedCSV(t, body)
	if len(got) != 1 || got[0] != "carol@spoof.example.com" {
		t.Fatalf("sender login (spoof): got %v, want [carol@spoof.example.com]", got)
	}

	if code, _ := doGet(t, app, "/stack/postfix/sender/login/nobody@unknown.example.com"); code != 404 {
		t.Fatalf("sender login unknown: got %d", code)
	}
}

// sortedCSV unmarshals a JSON string and returns its comma-separated values sorted.
func sortedCSV(t *testing.T, body string) []string {
	t.Helper()
	var s string
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unmarshal %q: %v", body, err)
	}
	parts := strings.Split(s, ",")
	sort.Strings(parts)
	return parts
}

func TestPostfixSRSAndRateContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	// Non-SRS recipients and served-domain senders are not rewritten.
	if code, _ := doGet(t, app, "/stack/postfix/recipient/map/alice@example.com"); code != 404 {
		t.Errorf("recipient/map plain: got %d, want 404", code)
	}
	if code, _ := doGet(t, app, "/stack/postfix/sender/map/alice@example.com"); code != 404 {
		t.Errorf("sender/map served domain: got %d, want 404", code)
	}

	// External sender is SRS-rewritten, then the recipient map restores it.
	code, body := doGet(t, app, "/stack/postfix/sender/map/john@example.org")
	if code != 200 || !strings.Contains(body, "SRS0=") {
		t.Fatalf("sender/map external: got %d %q", code, body)
	}
	var srs string
	if err := json.Unmarshal([]byte(body), &srs); err != nil {
		t.Fatalf("sender/map unmarshal: %v", err)
	}
	code, body = doGet(t, app, "/stack/postfix/recipient/map/"+srs)
	if code != 200 || body != `"john@example.org"` {
		t.Fatalf("recipient/map SRS: got %d %q", code, body)
	}

	// Rate endpoint answers 404 while under the limit (postfix treats a hit
	// as a temporary failure only when the JSON smtp code is returned).
	if code, _ := doGet(t, app, "/stack/postfix/sender/rate/alice@example.com"); code != 404 {
		t.Errorf("sender/rate under limit: got %d, want 404", code)
	}
	if code, _ := doGet(t, app, "/stack/postfix/sender/rate/nobody@example.com"); code != 404 {
		t.Errorf("sender/rate unknown: got %d, want 404", code)
	}
}

func TestDovecotContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	if code, body := doGet(t, app, "/stack/dovecot/passdb/alice@example.com"); code != 200 ||
		body != `{"allow_real_nets":"192.168.206.0/24","nopassword":"Y","password":null}` {
		t.Fatalf("passdb: got %d %q", code, body)
	}
	if code, body := doGet(t, app, "/stack/dovecot/userdb/alice@example.com"); code != 200 ||
		body != `{"quota_rule":"*:bytes=1000000000"}` {
		t.Fatalf("userdb: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/dovecot/passdb/nobody@example.com"); code != 404 {
		t.Fatalf("passdb missing: got %d", code)
	}

	// userdb iteration: set equality (SQL order is not guaranteed).
	code, body := doGet(t, app, "/stack/dovecot/userdb/")
	if code != 200 {
		t.Fatalf("userdb list: got %d", code)
	}
	var emails []string
	if err := json.Unmarshal([]byte(body), &emails); err != nil {
		t.Fatalf("userdb list unmarshal: %v", err)
	}
	sort.Strings(emails)
	if strings.Join(emails, ",") != "alice@example.com,bob@example.com,carol@spoof.example.com" {
		t.Fatalf("userdb list: got %v", emails)
	}

	// quota report is persisted.
	if code, body := doPost(t, app, "/stack/dovecot/quota/bytes/alice@example.com", "2048"); code != 200 {
		t.Fatalf("quota report: got %d %q", code, body)
	}
	var u models.User
	if err := h.DB.First(&u, "email = ?", "alice@example.com").Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if u.QuotaBytesUsed != 2048 {
		t.Fatalf("quota persisted: got %d", u.QuotaBytesUsed)
	}

	if code, body := doGet(t, app, "/stack/dovecot/sieve/name/default/alice@example.com"); code != 200 || body != `"default"` {
		t.Fatalf("sieve name: got %d %q", code, body)
	}
	code, body = doGet(t, app, "/stack/dovecot/sieve/data/default/alice@example.com")
	if code != 200 {
		t.Fatalf("sieve data: got %d", code)
	}
	var script string
	if err := json.Unmarshal([]byte(body), &script); err != nil {
		t.Fatalf("sieve data unmarshal: %v", err)
	}
	if !strings.Contains(script, `require "vacation";`) || !strings.Contains(script, `fileinto :create "Junk";`) {
		t.Fatalf("sieve data missing expected rules:\n%s", script)
	}
}

// TestDovecotSieveWhitelistBlacklist pins the generated sieve rules for a
// user with whitelist/blacklist entries (mixed domains and full addresses).
func TestDovecotSieveWhitelistBlacklist(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	var u models.User
	if err := h.DB.First(&u, "email = ?", "alice@example.com").Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if err := h.DB.Model(&u).Updates(map[string]any{
		"whitelist": "trusted.example.com, friend@example.net",
		"blacklist": "spam.example.com, bad@example.net",
	}).Error; err != nil {
		t.Fatalf("set lists: %v", err)
	}

	_, body := doGet(t, app, "/stack/dovecot/sieve/data/default/alice@example.com")
	var script string
	if err := json.Unmarshal([]byte(body), &script); err != nil {
		t.Fatalf("sieve data unmarshal: %v", err)
	}
	for _, want := range []string{
		`address :domain :is "From" "trusted.example.com"`,
		`address :is "From" "friend@example.net"`,
		`address :domain :is "From" "spam.example.com"`,
		`address :is "From" "bad@example.net"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("sieve data missing %q:\n%s", want, script)
		}
	}
	// whitelist must precede the spam filter so it bypasses Junk filing.
	if idxW, idxS := strings.Index(script, "address :domain :is \"From\" \"trusted.example.com\""), strings.Index(script, "if spamtest"); idxW < 0 || idxS < 0 || idxW > idxS {
		t.Fatalf("whitelist rule must appear before spamtest:\n%s", script)
	}
}

func TestRspamdContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	type selector struct {
		Domain   string `json:"domain"`
		Key      string `json:"key"`
		Selector string `json:"selector"`
	}
	var vault struct {
		Data struct {
			Selectors []selector `json:"selectors"`
		} `json:"data"`
	}

	// A domain with a DKIM key exposes exactly one selector.
	code, body := doGet(t, app, "/stack/rspamd/vault/v1/dkim/example.com")
	if code != 200 || json.Unmarshal([]byte(body), &vault) != nil {
		t.Fatalf("dkim vault: got %d %q", code, body)
	}
	if len(vault.Data.Selectors) != 1 {
		t.Fatalf("dkim vault selectors: got %d", len(vault.Data.Selectors))
	}
	s := vault.Data.Selectors[0]
	if s.Domain != "example.com" || s.Selector != "dkim" || s.Key != testDkimKey {
		t.Fatalf("dkim vault selector: got %+v", s)
	}

	// An alternative inherits the canonical key but advertises its own name.
	code, body = doGet(t, app, "/stack/rspamd/vault/v1/dkim/alt.example.com")
	if code != 200 || json.Unmarshal([]byte(body), &vault) != nil {
		t.Fatalf("dkim vault alternative: got %d %q", code, body)
	}
	if len(vault.Data.Selectors) != 1 {
		t.Fatalf("dkim vault alternative selectors: got %d", len(vault.Data.Selectors))
	}
	s = vault.Data.Selectors[0]
	if s.Domain != "alt.example.com" || s.Key != testDkimKey || s.Selector != "dkim" {
		t.Fatalf("dkim vault alternative selector: got %+v", s)
	}

	// Unknown domains still answer 200 with an empty selector list.
	code, body = doGet(t, app, "/stack/rspamd/vault/v1/dkim/unknown.example.com")
	if code != 200 || json.Unmarshal([]byte(body), &vault) != nil || len(vault.Data.Selectors) != 0 {
		t.Fatalf("dkim vault unknown: got %d %q", code, body)
	}

	// local_domains is plain text, one name per line (domains + alternatives).
	code, body = doGet(t, app, "/stack/rspamd/local_domains")
	if code != 200 {
		t.Fatalf("local_domains: got %d", code)
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	sort.Strings(lines)
	if strings.Join(lines, ",") != "alt.example.com,example.com,spoof.example.com" {
		t.Fatalf("local_domains: got %v", lines)
	}
}

func TestIDNAContract(t *testing.T) {
	h, app := newContractHarness(t)
	db := h.DB
	for _, v := range []any{
		&models.Domain{Name: "example.com"},
		&models.Domain{Name: "bücher.example", DkimKey: testDkimKey},
		&models.Alias{
			Email: "team@example.com", Localpart: "team", DomainName: "example.com",
			Destination: "x@bücher.example",
		},
	} {
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed %T: %v", v, err)
		}
	}

	unicode := url.PathEscape("bücher.example")
	punycode := "xn--bcher-kva.example"

	// Domain map answers punycode regardless of how the domain was stored or
	// queried.
	if code, body := doGet(t, app, "/stack/postfix/domain/"+punycode); code != 200 || body != `"`+punycode+`"` {
		t.Fatalf("domain punycode: got %d %q", code, body)
	}
	if code, body := doGet(t, app, "/stack/postfix/domain/"+unicode); code != 200 || body != `"`+punycode+`"` {
		t.Fatalf("domain unicode: got %d %q", code, body)
	}

	// rspamd vault advertises the punycode selector domain (mail-stack parity).
	for _, q := range []string{punycode, unicode} {
		code, body := doGet(t, app, "/stack/rspamd/vault/v1/dkim/"+q)
		if code != 200 {
			t.Fatalf("dkim vault %s: got %d", q, code)
		}
		var vault struct {
			Data struct {
				Selectors []struct {
					Domain string `json:"domain"`
				} `json:"selectors"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &vault); err != nil {
			t.Fatalf("dkim vault unmarshal: %v", err)
		}
		if len(vault.Data.Selectors) != 1 || vault.Data.Selectors[0].Domain != punycode {
			t.Fatalf("dkim vault %s: got %q", q, body)
		}
	}

	// Alias destinations are IDNA-encoded on the wire.
	if code, body := doGet(t, app, "/stack/postfix/alias/team@example.com"); code != 200 ||
		body != `"x@`+punycode+`"` {
		t.Fatalf("alias destination: got %d %q", code, body)
	}

	// local_domains passes through the stored names (mail-stack behaviour).
	code, body := doGet(t, app, "/stack/rspamd/local_domains")
	if code != 200 || !strings.Contains(body, "bücher.example") {
		t.Fatalf("local_domains: got %d %q", code, body)
	}
}

func TestFetchContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	type fetchAccount struct {
		ID        uint   `json:"id"`
		TLS       bool   `json:"tls"`
		Keep      bool   `json:"keep"`
		Scan      bool   `json:"scan"`
		Invisible bool   `json:"invisible"`
		UserEmail string `json:"user_email"`
		Protocol  string `json:"protocol"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		Folders   string `json:"folders"`
		Username  string `json:"username"`
		Password  string `json:"password"`
	}

	code, body := doGet(t, app, "/stack/fetch")
	if code != 200 {
		t.Fatalf("fetch list: got %d", code)
	}
	var accounts []fetchAccount
	if err := json.Unmarshal([]byte(body), &accounts); err != nil {
		t.Fatalf("fetch list unmarshal: %v (%q)", err, body)
	}
	if len(accounts) != 1 {
		t.Fatalf("fetch list: got %d accounts", len(accounts))
	}
	f := accounts[0]
	if f.ID != 1 || !f.TLS || !f.Keep || f.Scan || f.Invisible ||
		f.UserEmail != "alice@example.com" || f.Protocol != "imap" ||
		f.Host != "imap.example.org" || f.Port != 993 || f.Folders != "INBOX" ||
		f.Username != "alice" || f.Password != "fetchpass" {
		t.Fatalf("fetch account: got %+v", f)
	}

	// The poller reports its run result back to /internal/fetch/<id>.
	code, body = doPost(t, app, "/stack/fetch/1", `"connection refused"`)
	if code != 200 {
		t.Fatalf("fetch done: got %d %q", code, body)
	}
	var reloaded models.Fetch
	if err := h.DB.First(&reloaded, "id = ?", 1).Error; err != nil {
		t.Fatalf("reload fetch: %v", err)
	}
	if reloaded.Error != "connection refused" || reloaded.LastCheck == nil {
		t.Fatalf("fetch done persisted: error=%q last_check=%v", reloaded.Error, reloaded.LastCheck)
	}

	if code, _ := doPost(t, app, "/stack/fetch/999", `"x"`); code != 404 {
		t.Fatalf("fetch done unknown id: got %d", code)
	}
}

func TestAutoconfigContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	code, body := doGet(t, app, "/stack/autoconfig/mozilla")
	if code != 200 || !strings.Contains(body, "mail.example.com") || !strings.Contains(body, "%EMAILDOMAIN%") {
		t.Fatalf("mozilla: got %d %q", code, body)
	}

	code, body = doGet(t, app, "/stack/autoconfig/microsoft.json?Protocol=Autodiscoverv1")
	if code != 200 || !strings.Contains(body, `"Protocol":"Autodiscoverv1"`) {
		t.Fatalf("microsoft.json: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/autoconfig/microsoft.json?Protocol=Other"); code != 404 {
		t.Fatalf("microsoft.json other protocol: got %d", code)
	}

	req := httptest.NewRequest(http.MethodPost, "/stack/autoconfig/microsoft",
		strings.NewReader(`<?xml version="1.0"?><Autodiscover><Request><EMailAddress>alice@example.com</EMailAddress></Request></Autodiscover>`))
	req.Header.Set("Content-Type", "application/xml")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("microsoft POST: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "<Type>IMAP</Type>") {
		t.Fatalf("microsoft POST: got %d %q", resp.StatusCode, string(b))
	}

	code, body = doGet(t, app, "/stack/autoconfig/apple")
	if code != 200 || !strings.Contains(body, "EmailTypeIMAP") {
		t.Fatalf("apple: got %d %q", code, body)
	}
}
