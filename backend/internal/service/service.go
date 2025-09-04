// Package service implements the annual technical-service entitlement
// (技术服务). It is deliberately decoupled from the software license: every
// build can carry a service certificate, so a free installation can still
// buy paid support, SLA and launch assistance.
//
// A service certificate is an offline Ed25519-signed payload (same vendor
// key as the license) carrying the licensee, the SLA tier and the service
// end date. It gates nothing in the software — it is support entitlement.
package service

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Tiers are the purchasable support levels.
const (
	TierStandard = "standard"
	TierPremium  = "premium"
)

// Entitlement is the signed payload of a service certificate.
type Entitlement struct {
	Version   int      `json:"v"`
	Licensee  string   `json:"licensee,omitempty"`
	Tier      string   `json:"tier"`
	StartedAt string   `json:"started_at,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	Features  []string `json:"features,omitempty"`
}

// ErrInvalid reports a certificate that failed verification or is malformed.
var ErrInvalid = errors.New("invalid service certificate")

// Manager holds the loaded entitlement; nil means no service subscribed.
type Manager struct {
	ent Entitlement
}

// Load reads MAILEZ_SERVICE_FILE / MAILEZ_SERVICE and verifies it. Without a
// configured certificate the manager is nil (no service). A configured path
// whose file does not exist counts as "not subscribed" (compose files ship
// the mount by default; operators add the file when they subscribe). A file
// that exists must always verify — never silently drop a real certificate.
func Load(file, inline string) (*Manager, error) {
	raw := strings.TrimSpace(inline)
	if raw == "" && file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			return nil, fmt.Errorf("service: read %s: %w", file, err)
		}
		raw = strings.TrimSpace(string(b))
	}
	if raw == "" {
		return nil, nil
	}
	ent, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	return &Manager{ent: ent}, nil
}

// Parse verifies and decodes "base64url(payload).base64url(signature)".
func Parse(envelope string) (Entitlement, error) {
	parts := strings.SplitN(envelope, ".", 2)
	if len(parts) != 2 {
		return Entitlement{}, fmt.Errorf("%w: malformed envelope", ErrInvalid)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Entitlement{}, fmt.Errorf("%w: payload encoding", ErrInvalid)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Entitlement{}, fmt.Errorf("%w: signature encoding", ErrInvalid)
	}
	if !ed25519.Verify(publicKey, payload, sig) {
		return Entitlement{}, fmt.Errorf("%w: signature does not verify", ErrInvalid)
	}
	var ent Entitlement
	if err := json.Unmarshal(payload, &ent); err != nil {
		return Entitlement{}, fmt.Errorf("%w: payload: %v", ErrInvalid, err)
	}
	if ent.Version != 1 {
		return Entitlement{}, fmt.Errorf("%w: unsupported version %d", ErrInvalid, ent.Version)
	}
	if ent.Tier != TierStandard && ent.Tier != TierPremium {
		return Entitlement{}, fmt.Errorf("%w: unsupported tier %q", ErrInvalid, ent.Tier)
	}
	return ent, nil
}

// Sign creates the certificate envelope with the given private key.
func Sign(ent Entitlement, priv ed25519.PrivateKey) (string, error) {
	payload, err := json.Marshal(ent)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Entitlement returns the loaded entitlement (zero value when nil manager).
func (m *Manager) Entitlement() Entitlement {
	if m == nil {
		return Entitlement{}
	}
	return m.ent
}

// IsActive reports whether the service is currently in force (dev-style
// certificates without an end date never expire).
func (m *Manager) IsActive(now time.Time) bool {
	if m == nil || m.ent.ExpiresAt == "" {
		return m != nil
	}
	exp, err := time.Parse(time.RFC3339, m.ent.ExpiresAt)
	return err == nil && exp.After(now)
}

// Status is the read-only snapshot surfaced to the admin console.
type Status struct {
	Licensee  string   `json:"licensee,omitempty"`
	Tier      string   `json:"tier"`
	StartedAt string   `json:"started_at,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	Valid     bool     `json:"valid"`
	Features  []string `json:"features,omitempty"`
}

// Status builds the snapshot at the given time.
func (m *Manager) Status(now time.Time) *Status {
	if m == nil {
		return nil
	}
	return &Status{
		Licensee:  m.ent.Licensee,
		Tier:      m.ent.Tier,
		StartedAt: m.ent.StartedAt,
		ExpiresAt: m.ent.ExpiresAt,
		Features:  m.ent.Features,
		Valid:     m.IsActive(now),
	}
}
