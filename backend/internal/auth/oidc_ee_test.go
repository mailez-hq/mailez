//go:build mailez_ee

package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

// fakeIdP is a minimal OpenID Connect provider: discovery, a code-for-token
// endpoint minting RS256 id_tokens, and the JWKS. The test pins the account
// the token is issued for (idpEmail) and, per flow, the nonce copied out of
// the stored state (standing in for the authorization round-trip).
type fakeIdP struct {
	srv           *httptest.Server
	priv          *rsa.PrivateKey
	idpEmail      string
	idpNonce      string
	emailVerified bool
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	p := &fakeIdP{priv: priv, emailVerified: true}
	pub := &priv.PublicKey
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 p.srv.URL,
			"authorization_endpoint": p.srv.URL + "/auth",
			"token_endpoint":         p.srv.URL + "/token",
			"jwks_uri":               p.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "form", http.StatusBadRequest)
			return
		}
		if got := r.PostForm.Get("grant_type"); got != "authorization_code" {
			http.Error(w, "grant_type", http.StatusBadRequest)
			return
		}
		if got := r.PostForm.Get("code"); got != "good-code" {
			http.Error(w, "code", http.StatusBadRequest)
			return
		}
		if got := r.PostForm.Get("client_id") + ":" + r.PostForm.Get("client_secret"); got != "cid:cs" {
			http.Error(w, "client", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id_token": p.mint(t)})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// mint signs an RS256 id_token with the current idpEmail/idpNonce claims.
func (p *fakeIdP) mint(t *testing.T) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{
		"iss":            p.srv.URL,
		"aud":            "cid",
		"sub":            "user-1",
		"exp":            time.Now().Add(5 * time.Minute).Unix(),
		"iat":            time.Now().Unix(),
		"nonce":          p.idpNonce,
		"email":          p.idpEmail,
		"email_verified": p.emailVerified,
	})
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.priv, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// oidcApp mounts RegisterOIDC with the fake IdP on a fresh fiber app.
func oidcApp(t *testing.T, mgr *Manager, idp *fakeIdP) *fiber.App {
	t.Helper()
	app := fiber.New()
	mgr.RegisterOIDC(app, OIDCConfig{
		Issuer: idp.srv.URL, ClientID: "cid", ClientSecret: "cs",
		RedirectURL: "https://mail.test/api/v1/sso/oidc/callback",
	})
	return app
}

// oidcStart runs the start half only, returning the state so the test can
// read the nonce the flow bound (the fake IdP mints tokens with it).
func oidcStart(t *testing.T, mgr *Manager, idp *fakeIdP, next string) string {
	t.Helper()
	app := oidcApp(t, mgr, idp)
	startURL := "/sso/oidc/start"
	if next != "" {
		startURL += "?next=" + url.QueryEscape(next)
	}
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, startURL, nil))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start status: %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("start location: %v", err)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatalf("start redirect lacks state: %q", loc)
	}
	raw, ok, err := mgr.Store.Get(context.Background(), oidcStatePrefix+state)
	if err != nil || !ok {
		t.Fatalf("state not stored: %v %v", ok, err)
	}
	nonce, _, _ := strings.Cut(raw, "\x00")
	idp.idpNonce = nonce
	return state
}

func errReason(resp *http.Response) string {
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return ""
	}
	return loc.Query().Get("sso_error")
}

func seedDisabledUser(t *testing.T, mgr *Manager) {
	t.Helper()
	// Enabled carries gorm default:true, so a plain Create would flip the
	// zero value back on — create, then disable explicitly.
	hash, _ := password.Hash("correct-horse")
	u := models.User{Email: "b@example.com", Localpart: "b", DomainName: "example.com", Password: hash}
	if err := mgr.DB.Create(&u).Error; err != nil {
		t.Fatalf("seed disabled user: %v", err)
	}
	if err := mgr.DB.Model(&models.User{}).Where("email = ?", "b@example.com").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}
}

func TestOIDCFullFlow(t *testing.T) {
	mgr, _ := newTestManager(t)
	idp := newFakeIdP(t)
	idp.idpEmail = "a@example.com"
	state := oidcStart(t, mgr, idp, "/mail")
	cb, err := oidcApp(t, mgr, idp).Test(httptest.NewRequest(http.MethodGet,
		"/sso/oidc/callback?code=good-code&state="+url.QueryEscape(state), nil))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if cb.StatusCode != http.StatusFound {
		t.Fatalf("callback status: %d", cb.StatusCode)
	}
	if got := cb.Header.Get("Location"); got != "/mail" {
		t.Fatalf("callback must land on next, got %q", got)
	}
	cookie := ""
	for _, c := range cb.Cookies() {
		if c.Name == mgr.SessionName {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("callback issued no session cookie")
	}
	user, err := mgr.UserFromSession(context.Background(), cookie)
	if err != nil || user == nil || user.Email != "a@example.com" {
		t.Fatalf("session must resolve to a@example.com: %v %v", user, err)
	}
}

func TestOIDCUnsafeNextFallsBackToRoot(t *testing.T) {
	mgr, _ := newTestManager(t)
	idp := newFakeIdP(t)
	idp.idpEmail = "a@example.com"
	state := oidcStart(t, mgr, idp, "https://evil.example/phish")
	cb, err := oidcApp(t, mgr, idp).Test(httptest.NewRequest(http.MethodGet,
		"/sso/oidc/callback?code=good-code&state="+url.QueryEscape(state), nil))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if got := cb.Header.Get("Location"); got != "/" {
		t.Fatalf("absolute next must fall back to /, got %q", got)
	}
}

func TestOIDCStateReplayRejected(t *testing.T) {
	mgr, _ := newTestManager(t)
	idp := newFakeIdP(t)
	idp.idpEmail = "a@example.com"
	state := oidcStart(t, mgr, idp, "")
	app := oidcApp(t, mgr, idp)
	cb, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/sso/oidc/callback?code=good-code&state="+url.QueryEscape(state), nil))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if errReason(cb) != "" {
		t.Fatalf("first login must succeed, bounced with %q", errReason(cb))
	}
	replay, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/sso/oidc/callback?code=good-code&state="+url.QueryEscape(state), nil))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if got := errReason(replay); got != "state" {
		t.Fatalf("replay must bounce with sso_error=state, got %q", got)
	}
}

func TestOIDCDisabledUserRejected(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedDisabledUser(t, mgr)
	idp := newFakeIdP(t)
	idp.idpEmail = "b@example.com"
	state := oidcStart(t, mgr, idp, "")
	cb, err := oidcApp(t, mgr, idp).Test(httptest.NewRequest(http.MethodGet,
		"/sso/oidc/callback?code=good-code&state="+url.QueryEscape(state), nil))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if got := errReason(cb); got != "unknown_account" {
		t.Fatalf("disabled user must bounce with unknown_account, got %q", got)
	}
}

func TestOIDCNonceMismatchRejected(t *testing.T) {
	mgr, _ := newTestManager(t)
	idp := newFakeIdP(t)
	idp.idpEmail = "a@example.com"
	state := oidcStart(t, mgr, idp, "")
	// Sabotage the nonce binding: the IdP now signs a token for a different
	// nonce than the one the state stored.
	idp.idpNonce = "attacker-nonce"
	cb, err := oidcApp(t, mgr, idp).Test(httptest.NewRequest(http.MethodGet,
		"/sso/oidc/callback?code=good-code&state="+url.QueryEscape(state), nil))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if got := errReason(cb); got != "token" {
		t.Fatalf("nonce mismatch must bounce with token, got %q", got)
	}
}

func TestOIDCSignatureVerification(t *testing.T) {
	mgr, _ := newTestManager(t)
	idp := newFakeIdP(t)
	idp.idpEmail = "a@example.com"
	oc := &oidcClient{mgr: mgr,
		cfg: OIDCConfig{Issuer: idp.srv.URL, ClientID: "cid", ClientSecret: "cs"},
		hc:  &http.Client{Timeout: 5 * time.Second}}
	oc.jwks = map[string]*rsa.PublicKey{"k1": &idp.priv.PublicKey}
	jwksURI := idp.srv.URL + "/jwks"
	if _, err := oc.verifyIDToken(context.Background(), jwksURI, idp.mint(t), idp.idpNonce); err != nil {
		t.Fatalf("genuine token must verify: %v", err)
	}
	tok := idp.mint(t)
	parts := strings.Split(tok, ".")
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sig[0] ^= 0xff
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)
	if _, err := oc.verifyIDToken(context.Background(), jwksURI, tampered, idp.idpNonce); err == nil {
		t.Fatal("tampered signature must not verify")
	}
}

func TestOIDCDisabledFeatureMountsNothing(t *testing.T) {
	mgr, _ := newTestManager(t)
	app := fiber.New()
	mgr.RegisterOIDC(app, OIDCConfig{Issuer: "", ClientID: "cid", ClientSecret: ""})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/sso/oidc/start", nil))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("incomplete config must mount no routes, got %d", resp.StatusCode)
	}
	if OIDCEnabled("", "cid", "cs") || OIDCEnabled("iss", "", "cs") || OIDCEnabled("iss", "cid", "") {
		t.Fatal("OIDCEnabled must require issuer+client+secret")
	}
}
