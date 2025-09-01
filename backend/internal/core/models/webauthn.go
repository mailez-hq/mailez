package models

import "time"

// WebauthnCredential stores one registered passkey (WebAuthn credential) for
// a user. The private key never leaves the authenticator; only the public
// key is persisted. Credentials enable passwordless sign-in through
// /sso/passkey/* — possession of the authenticator replaces the password.
type WebauthnCredential struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserEmail    string     `gorm:"size:255;not null;index" json:"user_email"`
	Name         string     `gorm:"size:120;not null;default:''" json:"name"`
	CredentialID string     `gorm:"size:512;not null;uniqueIndex" json:"credential_id"`
	PublicKey    []byte     `gorm:"not null" json:"-"`
	Attestation  string     `gorm:"size:64" json:"attestation_type"`
	Transports   string     `gorm:"size:255" json:"transports"`
	AAGUID       string     `gorm:"size:64" json:"aaguid"`
	SignCount    uint32     `gorm:"not null;default:0" json:"sign_count"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	CreatedAt    time.Time  `json:"created_at"`
}
