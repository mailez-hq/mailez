// Package license implements offline signed licensing for the Mailez
// enterprise edition. A license is a signed payload (Ed25519) issued by
// Mailez HQ that carries the edition, the licensed mailbox count (用户数)
// and the annual service end date; the vendor public key is embedded in the
// binary.
//
// Business model: the software usage right is PERPETUAL — the mailbox cap is
// enforced forever. The service (support + version upgrades) is billed
// annually; the service end date only gates support/upgrade entitlement and
// is surfaced to the admin console. An expired service never stops the
// system or blocks mailbox creation within the licensed cap.
//
// Without a license the system runs in the built-in dev edition (unlimited),
// so open-source builds and local development keep working untouched. When a
// license file is provided it must verify; when MAILEZ_LICENSE_REQUIRED is
// set, a missing or invalid license refuses startup.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// Edition values. "dev" is the built-in unlimited development edition.
const (
	EditionDev        = "dev"
	EditionEnterprise = "enterprise"
	// EditionCommunity marks the free community edition (postdove engine):
	// no license is loaded, required or enforced.
	EditionCommunity = "community"
)

// License is the signed payload carried by a license file. ExpiresAt is the
// annual SERVICE end date (support/upgrades), not a usage expiry: the
// perpetual usage right and the mailbox cap stay valid after it passes.
type License struct {
	Version      int      `json:"v"`
	Edition      string   `json:"edition"`
	Licensee     string   `json:"licensee,omitempty"`
	MaxMailboxes int      `json:"max_mailboxes"`
	IssuedAt     string   `json:"issued_at,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Features     []string `json:"features,omitempty"`

	// signature is kept out of the JSON payload.
	signature []byte
}

// Manager loads and validates the license and answers capacity questions.
type Manager struct {
	lic      License
	required bool
}

// ErrInvalid reports a license that failed verification or is malformed.
var ErrInvalid = errors.New("invalid license")

// DevLicense is the built-in unlimited development license. It is never
// signed with the vendor key — the public key check accepts the "dev"
// edition as a first-class development mode.
func DevLicense() License {
	return License{
		Version:      1,
		Edition:      EditionDev,
		Licensee:     "development",
		MaxMailboxes: 0, // 0 = unlimited
	}
}

// Community returns the community-edition manager. The community edition is
// free: it never loads, requires or enforces a license, and reports no
// license on the admin overview.
func Community() *Manager {
	return &Manager{lic: License{Version: 1, Edition: EditionCommunity}}
}

// Load builds a Manager from MAILEZ_LICENSE_FILE / MAILEZ_LICENSE contents.
// When required is true a missing or invalid license is an error; otherwise
// the dev edition is used as fallback.
func Load(file, inline string, required bool) (*Manager, error) {
	raw := strings.TrimSpace(inline)
	if raw == "" && file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			if required {
				return nil, fmt.Errorf("license: read %s: %w", file, err)
			}
			return &Manager{lic: DevLicense()}, nil
		}
		raw = strings.TrimSpace(string(b))
	}
	if raw == "" {
		if required {
			return nil, fmt.Errorf("%w: no license configured (set MAILEZ_LICENSE_FILE)", ErrInvalid)
		}
		return &Manager{lic: DevLicense()}, nil
	}
	lic, err := Parse(raw)
	if err != nil {
		if required {
			return nil, err
		}
		return nil, fmt.Errorf("license: %w (MAILEZ_LICENSE_REQUIRED is off; refusing to fall back to dev on an explicitly provided license)", err)
	}
	return &Manager{lic: lic, required: required}, nil
}

// Parse verifies and decodes a license envelope
// "base64url(payload).base64url(signature)".
func Parse(envelope string) (License, error) {
	parts := strings.SplitN(envelope, ".", 2)
	if len(parts) != 2 {
		return License{}, fmt.Errorf("%w: malformed envelope", ErrInvalid)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return License{}, fmt.Errorf("%w: payload encoding: %v", ErrInvalid, err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return License{}, fmt.Errorf("%w: signature encoding", ErrInvalid)
	}
	if !ed25519.Verify(publicKey, payload, sig) {
		return License{}, fmt.Errorf("%w: signature does not verify", ErrInvalid)
	}
	var lic License
	if err := json.Unmarshal(payload, &lic); err != nil {
		return License{}, fmt.Errorf("%w: payload: %v", ErrInvalid, err)
	}
	lic.signature = sig
	if lic.Version != 1 {
		return License{}, fmt.Errorf("%w: unsupported version %d", ErrInvalid, lic.Version)
	}
	if lic.Edition != EditionEnterprise {
		return License{}, fmt.Errorf("%w: unsupported edition %q", ErrInvalid, lic.Edition)
	}
	return lic, nil
}

// Sign creates the license envelope with the given private key.
func Sign(lic License, priv ed25519.PrivateKey) (string, error) {
	payload, err := json.Marshal(lic)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// License returns the loaded license.
func (m *Manager) License() License { return m.lic }

// Edition returns the license edition ("dev" or "enterprise").
func (m *Manager) Edition() string { return m.lic.Edition }

// MaxMailboxes returns the licensed mailbox count; 0 means unlimited.
func (m *Manager) MaxMailboxes() int { return m.lic.MaxMailboxes }

// ExpiresAt parses the license expiry; zero time means no expiry.
func (m *Manager) ExpiresAt() (time.Time, error) {
	if m.lic.ExpiresAt == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, m.lic.ExpiresAt)
}

// IsEnterprise reports whether the loaded license is the paid edition.
func (m *Manager) IsEnterprise() bool { return m.lic.Edition == EditionEnterprise }

// IsCommunity reports whether the free community edition is active.
func (m *Manager) IsCommunity() bool { return m.lic.Edition == EditionCommunity }

// ServiceExpired reports whether the annual service (support/upgrades) is
// past its end date. The dev edition never expires. A true result does not
// stop the system — the perpetual usage right remains.
func (m *Manager) ServiceExpired(now time.Time) bool {
	if m.lic.Edition != EditionEnterprise || m.lic.ExpiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, m.lic.ExpiresAt)
	return err != nil || !exp.After(now)
}

// CheckCapacity verifies the license allows creating one more mailbox.
// Enterprise licenses are capped by MaxMailboxes (perpetual); the dev
// edition and unlimited licenses pass. Service expiry never blocks
// provisioning within the licensed cap.
func (m *Manager) CheckCapacity(db *gorm.DB) error {
	if !m.IsEnterprise() {
		return nil
	}
	if m.lic.MaxMailboxes <= 0 {
		return nil // 0 = unlimited
	}
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("license capacity: %w", err)
	}
	if count >= int64(m.lic.MaxMailboxes) {
		return fmt.Errorf("license limit reached: %d/%d mailboxes (用户数)", count, m.lic.MaxMailboxes)
	}
	return nil
}

// Status is the read-only license snapshot surfaced to the admin API.
type Status struct {
	Edition      string   `json:"edition"`
	Licensee     string   `json:"licensee,omitempty"`
	MaxMailboxes int      `json:"max_mailboxes"`
	Used         int64    `json:"used"`
	// ExpiresAt is the annual service end date (support/upgrades). The
	// perpetual usage right is unaffected after it passes.
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Valid        bool     `json:"valid"` // license signature/edition valid
	ServiceValid bool     `json:"service_valid"`
	Features     []string `json:"features,omitempty"`
	Required     bool     `json:"required"`
}

// Status builds the current license snapshot (counts users for the "used"
// field; 0 = unlimited).
func (m *Manager) Status(db *gorm.DB) Status {
	st := Status{
		Edition:      m.lic.Edition,
		Licensee:     m.lic.Licensee,
		MaxMailboxes: m.lic.MaxMailboxes,
		ExpiresAt:    m.lic.ExpiresAt,
		Features:     m.lic.Features,
		Required:     m.required,
		Valid:        true,
		ServiceValid: !m.ServiceExpired(time.Now()),
	}
	if st.MaxMailboxes > 0 {
		_ = db.Model(&models.User{}).Count(&st.Used)
	}
	return st
}
