package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/gorm"

	"mailess/backend/internal/models"
	"mailess/backend/internal/password"
)

// Manager coordinates sessions, login and temporary tokens.
type Manager struct {
	Store       Store
	DB          *gorm.DB
	SessionName string
	SessionTTL  time.Duration
	TokenTTL    time.Duration
}

const sessionKeyPrefix = "mailess:session:"
const tokenKeyPrefix = "mailess:token:"

// NewManager wires the auth manager. store may be nil (disabled sessions).
func NewManager(db *gorm.DB, store Store, sessionName string, ttl time.Duration) *Manager {
	return &Manager{
		Store:       store,
		DB:          db,
		SessionName: sessionName,
		SessionTTL:  ttl,
		TokenTTL:    ttl,
	}
}

// Login validates credentials and creates a session, returning the session id.
func (m *Manager) Login(ctx context.Context, email, pw string) (string, *models.User, error) {
	var user models.User
	if err := m.DB.First(&user, "email = ?", email).Error; err != nil {
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
	var user models.User
	if err := m.DB.First(&user, "email = ?", email).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Logout invalidates a session.
func (m *Manager) Logout(ctx context.Context, sid string) error {
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
