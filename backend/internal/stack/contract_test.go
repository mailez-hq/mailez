package stack

// Contract tests for the /stack API consumed by the mail images (nginx /
// rspamd) and the mailezine engine. The response formats below must stay
// byte-compatible with the internal API contract, so these tests pin the
// exact wire contract (status codes, JSON quoting, list formatting).

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
	"mailez/backend/internal/authcache"
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
		MailEngine:         "mailezine",
		MailImapAddr:       "127.0.0.1:143",
		MailSmtpAddr:       "127.0.0.1:1587",
		MailSieveAddr:      "127.0.0.1:4190",
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	h := New(db, mgr, cfg, nil, nil, authcache.New(0))
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

	// Unknown users are flagged for the mail proxy.
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


	// rspamd vault advertises the punycode selector domain (image parity).
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


	// local_domains passes through the stored names (image behaviour).
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

func TestDirectoryContract(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	// --- users ---
	code, body := doGet(t, app, "/stack/directory/users/alice@example.com")
	if code != 200 || !strings.Contains(body, `"email":"alice@example.com"`) || !strings.Contains(body, `"quotaBytes":1000000000`) {
		t.Fatalf("directory users: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/directory/users/nobody@example.com"); code != 404 {
		t.Fatalf("directory users missing: got %d", code)
	}

	// --- domains (canonical + alternative) ---
	if code, body := doGet(t, app, "/stack/directory/domains/example.com"); code != 200 || !strings.Contains(body, `"isLocal":true`) {
		t.Fatalf("directory domains: got %d %q", code, body)
	}
	if code, body := doGet(t, app, "/stack/directory/domains/alt.example.com"); code != 200 || !strings.Contains(body, `"name":"example.com"`) {
		t.Fatalf("directory domains alternative: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/directory/domains/unknown.org"); code != 404 {
		t.Fatalf("directory domains missing: got %d", code)
	}

	// --- aliases (exact + wildcard) ---
	code, body = doGet(t, app, "/stack/directory/aliases/team@example.com")
	if code != 200 || !strings.Contains(body, "alice@example.com") || !strings.Contains(body, "bob@example.com") {
		t.Fatalf("directory aliases exact: got %d %q", code, body)
	}
	code, body = doGet(t, app, "/stack/directory/aliases/random@example.com")
	if code != 200 || !strings.Contains(body, `"targets":["alice@example.com"]`) {
		t.Fatalf("directory aliases wildcard: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/directory/aliases/nobody@unknown.org"); code != 404 {
		t.Fatalf("directory aliases missing: got %d", code)
	}

	// --- relays ---
	code, body = doGet(t, app, "/stack/directory/relays/foo@relay.example.com")
	if code != 200 || !strings.Contains(body, `"transport":"smtp:[relay.example.com]:2525"`) {
		t.Fatalf("directory relays: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/directory/relays/foo@example.com"); code != 404 {
		t.Fatalf("directory relays missing: got %d", code)
	}

	// --- senders (user + spoofing) ---
	if code, body := doGet(t, app, "/stack/directory/senders/alice@example.com"); code != 200 || !strings.Contains(body, `"allowed":true`) {
		t.Fatalf("directory senders user: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/directory/senders/nobody@unknown.org"); code != 404 {
		t.Fatalf("directory senders missing: got %d", code)
	}
	if code, body := doGet(t, app, "/stack/directory/senders/alice@example.com/rate"); code != 200 || !strings.Contains(body, `"allowed":true`) {
		t.Fatalf("directory sender rate: got %d %q", code, body)
	}

	// --- quota read + update ---
	if code, body := doGet(t, app, "/stack/directory/quota/alice@example.com"); code != 200 || !strings.Contains(body, `"limit":1000000000`) {
		t.Fatalf("directory quota: got %d %q", code, body)
	}
	if code, _ := doPost(t, app, "/stack/directory/quota/alice@example.com", `12345`); code != 200 {
		t.Fatalf("directory quota update: got %d", code)
	}
	if code, body := doGet(t, app, "/stack/directory/quota/alice@example.com"); code != 200 || !strings.Contains(body, `"used":12345`) {
		t.Fatalf("directory quota after update: got %d %q", code, body)
	}

	// --- sieve ---
	if code, body := doGet(t, app, "/stack/directory/sieve/alice@example.com"); code != 200 || !strings.Contains(body, `"name":"default"`) || !strings.Contains(body, "require") {
		t.Fatalf("directory sieve: got %d %q", code, body)
	}

	// --- SRS forward + restore round-trip ---
	code, body = doGet(t, app, "/stack/directory/srs/external@other.org")
	if code != 200 {
		t.Fatalf("directory srs forward: got %d", code)
	}
	var fwd map[string]string
	if err := json.Unmarshal([]byte(body), &fwd); err != nil || !strings.Contains(fwd["rewritten"], ".SRS0=") {
		t.Fatalf("directory srs forward: got %q err=%v", body, err)
	}
	code, body = doGet(t, app, "/stack/directory/srs/restore/"+url.PathEscape(fwd["rewritten"]))
	if code != 200 || !strings.Contains(body, `"original":"external@other.org"`) {
		t.Fatalf("directory srs restore: got %d %q", code, body)
	}
	if code, _ := doGet(t, app, "/stack/directory/srs/restore/alice@example.com"); code != 404 {
		t.Fatalf("directory srs restore non-srs: got %d", code)
	}
}

// TestDistributionGroupExpansion pins the 閫氳缁?delivery contract: structured
// members join the destination list, nested local groups expand recursively,
// cycles terminate, and local users / external members pass through.
func TestDistributionGroupExpansion(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)
	db := h.DB

	// eng is a distribution group with a local user, another local user, and
	// an external member.
	eng := &models.Alias{Email: "eng@example.com", Localpart: "eng", DomainName: "example.com"}
	eng.SetMembers([]models.AliasMember{
		{Email: "alice@example.com", Name: "Alice"},
		{Email: "bob@example.com"},
		{Email: "partner@elsewhere.com"},
	})
	if err := db.Create(eng).Error; err != nil {
		t.Fatalf("create eng: %v", err)
	}

	// all nests eng plus a direct member; then a cycle eng -> all is added to
	// prove the resolver terminates.
	all := &models.Alias{Email: "all@example.com", Localpart: "all", DomainName: "example.com"}
	all.SetMembers([]models.AliasMember{
		{Email: "eng@example.com"},
		{Email: "carol@spoof.example.com"},
	})
	if err := db.Create(all).Error; err != nil {
		t.Fatalf("create all: %v", err)
	}
	eng.SetMembers(append(eng.MemberList(), models.AliasMember{Email: "all@example.com"}))
	if err := db.Save(eng).Error; err != nil {
		t.Fatalf("save eng cycle: %v", err)
	}

	code, body := doGet(t, app, "/stack/directory/aliases/all@example.com")
	if code != 200 {
		t.Fatalf("directory aliases: got %d %q", code, body)
	}
	var res struct {
		Targets []string `json:"targets"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("unmarshal: %v (%q)", err, body)
	}
	got := map[string]bool{}
	for _, tgt := range res.Targets {
		got[strings.ToLower(tgt)] = true
	}
	for _, want := range []string{
		"alice@example.com", "bob@example.com",
		"partner@elsewhere.com", "carol@spoof.example.com",
	} {
		if !got[want] {
			t.Fatalf("target %s missing from %v", want, res.Targets)
		}
	}
	if len(res.Targets) != 4 {
		t.Fatalf("targets = %v, want 4 unique entries", res.Targets)
	}
}
