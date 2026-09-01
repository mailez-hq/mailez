package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
	"mailez/backend/internal/webauthn"
)

// LDAPAuthenticator is the directory integration surface the auth manager
// falls back to when local credentials do not match.
type LDAPAuthenticator interface {
	Authenticate(ctx context.Context, email, password string) (bool, error)
	EnsureLocalUser(ctx context.Context, email string) error
}

// Manager coordinates sessions, login and temporary tokens.
type Manager struct {
	Store       Store
	DB          *gorm.DB
	SessionName string
	SessionTTL  time.Duration
	TokenTTL    time.Duration
	LDAP        LDAPAuthenticator
	// WebAuthn, when wired, enables passwordless passkey sign-in through
	// /sso/passkey/*. Nil keeps the routes answering 404 (feature off).
	WebAuthn *webauthn.Service

	secureCookie bool
	loginWindow  time.Duration
	loginPerIP   int
	loginFail    int

	// NotifyLogin, when set, is called after a successful password login from
	// an IP the account has not used before. The server wires it to the
	// security-alert email delivery.
	NotifyLogin func(email, ip, userAgent string)
}

const sessionKeyPrefix = "mailez:session:"
const tokenKeyPrefix = "mailez:token:"
const sessionTokenPrefix = "mailez:session-token:"
const pending2faPrefix = "mailez:pending2fa:"
const totpFailPrefix = "mailez:totpfail:"

// NewManager wires the auth manager. store may be nil (disabled sessions).
func NewManager(db *gorm.DB, store Store, sessionName string, ttl time.Duration) *Manager {
	return &Manager{
		Store:       store,
		DB:          db,
		SessionName: sessionName,
		SessionTTL:  ttl,
		TokenTTL:    ttl,
		loginWindow: 15 * time.Minute,
		loginPerIP:  30,
		loginFail:   10,
	}
}

// SetCookieSecure marks session cookies Secure (required behind HTTPS).
func (m *Manager) SetCookieSecure(v bool) { m.secureCookie = v }

// SetLoginLimits overrides the login brute-force limits (per IP per window,
// and per-email failure lockout). Values <= 0 keep the defaults.
func (m *Manager) SetLoginLimits(perIP, fail int) {
	if perIP > 0 {
		m.loginPerIP = perIP
	}
	if fail > 0 {
		m.loginFail = fail
	}
}

// RateAllow is a generic fixed-window counter over the auth store so other
// public endpoints (e.g. self-signup) can share the login rate-limiting
// machinery: it reports whether key under scope is still under limit within
// window. A nil store (disabled sessions) or a store error fails open, and
// limit <= 0 disables the check — mirroring checkLoginAttempt semantics.
func (m *Manager) RateAllow(ctx context.Context, scope, key string, limit int, window time.Duration) bool {
	if m.Store == nil || limit <= 0 {
		return true
	}
	return m.incr(ctx, "mailez:rate:"+scope+":"+key, window) <= limit
}

// rememberLoginIP tracks the IP the account logged in from; it returns true
// when this IP is new (the caller should raise a security alert).
func (m *Manager) rememberLoginIP(ctx context.Context, email, ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	var user models.User
	if err := m.DB.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return false
	}
	known := splitKnownIPs(user.KnownIPs)
	for _, k := range known {
		if k == ip {
			return false
		}
	}
	known = append(known, ip)
	if len(known) > 24 {
		known = known[len(known)-24:]
	}
	_ = m.DB.Model(&models.User{}).Where("email = ?", email).
		Update("known_ips", strings.Join(known, ",")).Error
	return true
}

func splitKnownIPs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Login validates credentials and creates a session, returning the session id.
func (m *Manager) Login(ctx context.Context, email, pw string) (string, *models.User, error) {
	var user models.User
	if err := m.DB.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return "", nil, err
	}
	if !user.Enabled || !password.Verify(user.Password, pw) {
		return "", nil, nil
	}
	sid, err := m.CreateSession(ctx, user.Email)
	return sid, &user, err
}

// CreateSession issues a new session id for the user.
func (m *Manager) CreateSession(ctx context.Context, email string) (string, error) {
	sid, err := randomToken(32)
	if err != nil {
		return "", err
	}
	if err := m.Store.Set(ctx, sessionKeyPrefix+sid, email, m.SessionTTL); err != nil {
		return "", err
	}
	return sid, nil
}

// UserFromSession returns the user for a session id, or nil.
func (m *Manager) UserFromSession(ctx context.Context, sid string) (*models.User, error) {
	email, ok, err := m.Store.Get(ctx, sessionKeyPrefix+sid)
	if err != nil || !ok {
		return nil, err
	}
	// Sliding expiry: refresh the session TTL on every authenticated access.
	if m.SessionTTL > 0 {
		_ = m.Store.Set(ctx, sessionKeyPrefix+sid, email, m.SessionTTL)
	}
	var user models.User
	if err := m.DB.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Logout invalidates a session and its stable temp token, so pooled or cached
// credentials tied to that session stop being accepted on the next login.
func (m *Manager) Logout(ctx context.Context, sid string) error {
	_ = m.Store.Delete(ctx, sessionTokenPrefix+sid)
	return m.Store.Delete(ctx, sessionKeyPrefix+sid)
}

// CreateTempToken stores a temporary token bound to the session. Webmail uses
// it as an IMAP/SMTP password without exposing the user's real password.
func (m *Manager) CreateTempToken(ctx context.Context, email, sid string) (string, error) {
	token, err := randomToken(24)
	if err != nil {
		return "", err
	}
	full := "token-" + token
	if err := m.Store.Set(ctx, tokenKeyPrefix+full, sid, m.TokenTTL); err != nil {
		return "", err
	}
	return full, nil
}

// SessionToken returns the session's stable temp token, minting it on first
// use and reusing it for the lifetime of the session. Reusing one credential
// keeps the engine-side authentication cache (keyed by email + token) hot:
// every webmail request after the first skips the control-plane auth round
// trip entirely. The token stays bound to the session — Logout deletes the
// mapping and VerifyTempToken re-checks the session on every login — so a
// rotated or logged-out session cannot ride a cached success.
func (m *Manager) SessionToken(ctx context.Context, email, sid string) (string, error) {
	if sid != "" {
		if tok, ok, _ := m.Store.Get(ctx, sessionTokenPrefix+sid); ok && tok != "" {
			// Re-validate before reuse: the token must still resolve back to
			// this session, otherwise a stale mapping (e.g. after an external
			// store evicted the token row) must not be replayed.
			if s, ok2, _ := m.Store.Get(ctx, tokenKeyPrefix+tok); ok2 && s == sid {
				return tok, nil
			}
			_ = m.Store.Delete(ctx, sessionTokenPrefix+sid)
		}
	}
	full, err := m.CreateTempToken(ctx, email, sid)
	if err != nil {
		return "", err
	}
	if sid != "" {
		if err := m.Store.Set(ctx, sessionTokenPrefix+sid, full, m.TokenTTL); err != nil {
			// The token itself is still valid; losing the mapping only means
			// the next call mints a fresh one.
			return full, nil
		}
	}
	return full, nil
}

// CreatePending2FA stores a short-lived token for the second-factor step of a
// login, so the password is never enough to open a session for a 2FA user.
func (m *Manager) CreatePending2FA(ctx context.Context, email string) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	if err := m.Store.Set(ctx, pending2faPrefix+token, email, 5*time.Minute); err != nil {
		return "", err
	}
	return token, nil
}

// ConsumePending2FA returns the email behind a pending token (one use only).
func (m *Manager) ConsumePending2FA(ctx context.Context, token string) (string, bool) {
	email, ok, err := m.Store.Get(ctx, pending2faPrefix+token)
	if err != nil || !ok {
		return "", false
	}
	_ = m.Store.Delete(ctx, pending2faPrefix+token)
	return email, true
}

// PeekPending2FA reads a pending token without consuming it, so a mistyped
// TOTP code does not force the user back to the password step.
func (m *Manager) PeekPending2FA(ctx context.Context, token string) (string, bool) {
	email, ok, err := m.Store.Get(ctx, pending2faPrefix+token)
	if err != nil || !ok {
		return "", false
	}
	return email, true
}

// totpMaxAttempts bounds how many codes may be tried against one pending
// token before the 2FA step is burned and the password must be re-entered.
const totpMaxAttempts = 5

// FailTotpAttempt records a wrong TOTP code against the pending token and
// consumes the token once the attempt budget is spent.
func (m *Manager) FailTotpAttempt(ctx context.Context, token string) bool {
	n, err := m.Store.Incr(ctx, totpFailPrefix+token, 5*time.Minute)
	if err != nil {
		// Fail closed: without a counter we cannot bound guessing.
		_ = m.Store.Delete(ctx, pending2faPrefix+token)
		return true
	}
	if n >= totpMaxAttempts {
		_ = m.Store.Delete(ctx, pending2faPrefix+token)
		_ = m.Store.Delete(ctx, totpFailPrefix+token)
		return true
	}
	return false
}

// VerifyTempToken checks a token-* credential against a user.
func (m *Manager) VerifyTempToken(ctx context.Context, email, token string) bool {
	if len(token) < 6 || token[:6] != "token-" {
		return false
	}
	sid, ok, err := m.Store.Get(ctx, tokenKeyPrefix+token)
	if err != nil || !ok {
		return false
	}
	sessionEmail, ok, err := m.Store.Get(ctx, sessionKeyPrefix+sid)
	if err != nil || !ok {
		return false
	}
	return sessionEmail == email
}

// SessionEmailFromToken resolves the session owner behind a temp token without
// requiring the credential to match a specific account. The mailbox delegation
// path uses it to allow a delegate's token to authenticate as the owner whose
// mailbox was delegated to them (full access).
func (m *Manager) SessionEmailFromToken(ctx context.Context, token string) (string, bool) {
	if len(token) < 6 || token[:6] != "token-" {
		return "", false
	}
	sid, ok, err := m.Store.Get(ctx, tokenKeyPrefix+token)
	if err != nil || !ok {
		return "", false
	}
	email, ok, err := m.Store.Get(ctx, sessionKeyPrefix+sid)
	if err != nil || !ok {
		return "", false
	}
	return email, true
}

// IsAppToken reports whether the credential looks like a 32-char hex app token.
func IsAppToken(candidate string) bool {
	if len(candidate) != 32 {
		return false
	}
	for _, c := range candidate {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
