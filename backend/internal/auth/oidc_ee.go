//go:build mailez_ee

package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// OIDCEnabled reports whether the OIDC federation settings are complete. It
// backs the public /server/settings "oidc.enabled" advertisement so the login
// pages only render the SSO button when the routes are actually mounted.
func OIDCEnabled(issuer, clientID, clientSecret string) bool {
	return issuer != "" && clientID != "" && clientSecret != ""
}

const (
	oidcStatePrefix = "mailez:oidc:state:"
	oidcStateTTL    = 10 * time.Minute
)

// RegisterOIDC mounts the federated sign-in routes (enterprise edition):
// GET /sso/oidc/start redirects to the provider's authorization endpoint and
// GET /sso/oidc/callback consumes the authorization code. With an incomplete
// configuration it mounts nothing.
func (m *Manager) RegisterOIDC(r fiber.Router, cfg OIDCConfig) {
	if m.Store == nil || !OIDCEnabled(cfg.Issuer, cfg.ClientID, cfg.ClientSecret) {
		return
	}
	oc := &oidcClient{mgr: m, cfg: cfg, hc: &http.Client{Timeout: 10 * time.Second}}
	r.Get("/sso/oidc/start", oc.start)
	r.Get("/sso/oidc/callback", oc.callback)
}

// oidcClient holds the per-deployment OIDC state: the frozen config plus the
// cached discovery document and JWKS (both re-fetched hourly).
type oidcClient struct {
	mgr *Manager
	cfg OIDCConfig
	hc  *http.Client

	mu      sync.Mutex
	disco   map[string]any
	discoAt time.Time
	jwks    map[string]*rsa.PublicKey
	jwksAt  time.Time
}

// start begins the authorization-code flow: mint a single-use state bound to
// a nonce (and the post-login landing path), then 302 to the provider.
func (oc *oidcClient) start(c *fiber.Ctx) error {
	if !oc.mgr.RateAllow(c.Context(), "oidc-start", c.IP(), 30, 15*time.Minute) {
		return oc.fail(c, "rate_limited")
	}
	state, err := randomToken(24)
	if err != nil {
		return oc.fail(c, "internal")
	}
	nonce, err := randomToken(16)
	if err != nil {
		return oc.fail(c, "internal")
	}
	authEP, _ := oc.discover(c.Context())["authorization_endpoint"].(string)
	if authEP == "" {
		return oc.fail(c, "discovery")
	}
	payload := nonce + "\x00" + safeNext(c.Query("next"))
	if err := oc.mgr.Store.Set(c.Context(), oidcStatePrefix+state, payload, oidcStateTTL); err != nil {
		return oc.fail(c, "internal")
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", oc.cfg.ClientID)
	q.Set("redirect_uri", oc.cfg.RedirectURL)
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("nonce", nonce)
	return c.Redirect(authEP+"?"+q.Encode(), fiber.StatusFound)
}

// callback finishes the flow: consume the state, exchange the code, verify
// the id_token (RS256, iss/aud/exp/nonce), match the verified e-mail against
// an existing enabled account and issue a session cookie.
func (oc *oidcClient) callback(c *fiber.Ctx) error {
	state, code := c.Query("state"), c.Query("code")
	if state == "" || code == "" {
		return oc.fail(c, "invalid_response")
	}
	raw, ok, err := oc.mgr.Store.Get(c.Context(), oidcStatePrefix+state)
	if err != nil || !ok {
		return oc.fail(c, "state")
	}
	_ = oc.mgr.Store.Delete(c.Context(), oidcStatePrefix+state) // single use: replay bounces
	nonce, next, ok := strings.Cut(raw, "\x00")
	if !ok {
		return oc.fail(c, "state")
	}
	claims, err := oc.exchange(c.Context(), code, nonce)
	if err != nil {
		return oc.fail(c, "token")
	}
	email := strings.ToLower(strings.TrimSpace(claimStr(claims["email"])))
	if email == "" {
		return oc.fail(c, "email")
	}
	// Unsigned, unverified addresses must never log anyone in.
	if verified, ok := claims["email_verified"].(bool); ok && !verified {
		return oc.fail(c, "email")
	}
	var user models.User
	// Match-only federation: no just-in-time provisioning, so an IdP account
	// can never conjure a local account out of thin air.
	if err := oc.mgr.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error; err != nil || !user.Enabled {
		return oc.fail(c, "unknown_account")
	}
	sid, err := oc.mgr.CreateSession(c.Context(), user.Email)
	if err != nil {
		return oc.fail(c, "internal")
	}
	if oc.mgr.NotifyLogin != nil && oc.mgr.rememberLoginIP(c.Context(), user.Email, c.IP()) {
		oc.mgr.NotifyLogin(user.Email, c.IP(), c.Get("User-Agent"))
	}
	setSessionCookie(c, oc.mgr.SessionName, sid, oc.mgr.SessionTTL, oc.mgr.secureCookie)
	oc.mgr.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "GET", Path: "/sso/oidc/callback", Status: fiber.StatusOK, Action: "login.oidc"})
	return c.Redirect(next, fiber.StatusFound)
}

// fail bounces the browser back to the login page with a coarse reason. The
// reasons are deliberately low-cardinality: they must be safe to show to an
// unauthenticated visitor.
func (oc *oidcClient) fail(c *fiber.Ctx, reason string) error {
	return c.Redirect("/?sso_error="+url.QueryEscape(reason), fiber.StatusFound)
}

// safeNext only trusts same-site relative paths for the post-login landing.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

func claimStr(v any) string {
	s, _ := v.(string)
	return s
}

func audMatches(aud any, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == clientID {
				return true
			}
		}
	}
	return false
}

// exchange trades the authorization code for an id_token (client_secret_post)
// and returns its verified claims.
func (oc *oidcClient) exchange(ctx context.Context, code, nonce string) (map[string]any, error) {
	d := oc.discover(ctx)
	tokenEP, _ := d["token_endpoint"].(string)
	jwksURI, _ := d["jwks_uri"].(string)
	if tokenEP == "" || jwksURI == "" {
		return nil, errors.New("oidc: discovery document lacks token_endpoint/jwks_uri")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", oc.cfg.RedirectURL)
	form.Set("client_id", oc.cfg.ClientID)
	form.Set("client_secret", oc.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEP, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oc.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: token endpoint status %d", resp.StatusCode)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("oidc: decode token reply: %w", err)
	}
	if tok.IDToken == "" {
		return nil, errors.New("oidc: token reply lacks id_token")
	}
	return oc.verifyIDToken(ctx, jwksURI, tok.IDToken, nonce)
}

// verifyIDToken checks the JWT envelope (RS256 only, to rule out algorithm
// confusion), resolves the signing key from the provider's JWKS and validates
// the claims the login decision rests on.
func (oc *oidcClient) verifyIDToken(ctx context.Context, jwksURI, token, nonce string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("oidc: malformed id_token")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("oidc: header: %w", err)
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(hb, &head); err != nil {
		return nil, fmt.Errorf("oidc: header: %w", err)
	}
	if head.Alg != "RS256" {
		return nil, fmt.Errorf("oidc: unexpected alg %q", head.Alg)
	}
	key, err := oc.signingKey(ctx, jwksURI, head.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("oidc: signature: %w", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return nil, fmt.Errorf("oidc: signature: %w", err)
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("oidc: payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(pb, &claims); err != nil {
		return nil, fmt.Errorf("oidc: payload: %w", err)
	}
	if strings.TrimSuffix(claimStr(claims["iss"]), "/") != strings.TrimSuffix(oc.cfg.Issuer, "/") {
		return nil, errors.New("oidc: issuer mismatch")
	}
	if !audMatches(claims["aud"], oc.cfg.ClientID) {
		return nil, errors.New("oidc: audience mismatch")
	}
	if exp, ok := claims["exp"].(float64); !ok || time.Now().Unix() > int64(exp)+60 {
		return nil, errors.New("oidc: token expired")
	}
	if subtle.ConstantTimeCompare([]byte(claimStr(claims["nonce"])), []byte(nonce)) != 1 {
		return nil, errors.New("oidc: nonce mismatch")
	}
	return claims, nil
}

// discover fetches (and caches for an hour) the provider's
// .well-known/openid-configuration document; a fetch error serves a stale
// copy when one exists so a provider blip does not break running logins.
func (oc *oidcClient) discover(ctx context.Context) map[string]any {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	if oc.disco != nil && time.Since(oc.discoAt) < time.Hour {
		return oc.disco
	}
	wellKnown := strings.TrimSuffix(oc.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	disco, err := fetchJSON(ctx, oc.hc, wellKnown)
	if err != nil {
		if oc.disco != nil {
			return oc.disco
		}
		return map[string]any{}
	}
	oc.disco = disco
	oc.discoAt = time.Now()
	return disco
}

// signingKey resolves the kid-named RSA key from the JWKS. An unknown kid
// against a fresh cache triggers exactly one forced refresh — providers can
// rotate keys at any time.
func (oc *oidcClient) signingKey(ctx context.Context, jwksURI, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("oidc: id_token missing kid")
	}
	oc.mu.Lock()
	keys, at := oc.jwks, oc.jwksAt
	oc.mu.Unlock()
	if keys == nil || time.Since(at) >= time.Hour {
		if keys = oc.refreshJWKS(ctx, jwksURI); keys == nil {
			return nil, errors.New("oidc: jwks unavailable")
		}
	}
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	if keys = oc.refreshJWKS(ctx, jwksURI); keys != nil {
		if key, ok := keys[kid]; ok {
			return key, nil
		}
	}
	return nil, fmt.Errorf("oidc: unknown signing key %q", kid)
}

func (oc *oidcClient) refreshJWKS(ctx context.Context, jwksURI string) map[string]*rsa.PublicKey {
	raw, err := fetchJSON(ctx, oc.hc, jwksURI)
	if err != nil {
		return nil
	}
	keys := map[string]*rsa.PublicKey{}
	list, _ := raw["keys"].([]any)
	for _, item := range list {
		jwk, _ := item.(map[string]any)
		if kty, _ := jwk["kty"].(string); kty != "RSA" {
			continue
		}
		if alg, _ := jwk["alg"].(string); alg != "" && alg != "RS256" {
			continue
		}
		kid, _ := jwk["kid"].(string)
		n, e := decodeBase64URLUint(jwk["n"]), decodeBase64URLUint(jwk["e"])
		if kid == "" || n == nil || e == nil || !e.IsInt64() {
			continue
		}
		keys[kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}
	if len(keys) > 0 {
		oc.mu.Lock()
		oc.jwks, oc.jwksAt = keys, time.Now()
		oc.mu.Unlock()
	}
	return keys
}

func decodeBase64URLUint(v any) *big.Int {
	s, _ := v.(string)
	if s == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	return new(big.Int).SetBytes(b)
}

func fetchJSON(ctx context.Context, hc *http.Client, rawURL string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: GET %s status %d", rawURL, resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("oidc: decode %s: %w", rawURL, err)
	}
	return out, nil
}
