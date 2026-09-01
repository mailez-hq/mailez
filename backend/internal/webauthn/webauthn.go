// Package webauthn wraps the W3C WebAuthn / FIDO2 registration and
// authentication ceremony (passkeys) on top of github.com/go-webauthn.
// Challenge/session state lives in the same store as sessions (Redis in
// production) so begin/finish may hit different backend replicas.
package webauthn

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// challengeTTL bounds how long a browser may take between the begin and
// finish calls of a ceremony.
const challengeTTL = 5 * time.Minute

// Store is the subset of the session store the service needs.
type Store interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, bool, error)
	Delete(ctx context.Context, key string) error
}

// Service performs WebAuthn ceremonies against the users and
// webauthn_credentials tables.
type Service struct {
	DB     *gorm.DB
	Store  Store
	RPID   string
	RPName string
	// Origins are the exact browser origins allowed to complete ceremonies
	// (scheme + host[:port]), e.g. https://mail.example.com.
	Origins []string

	wa *webauthn.WebAuthn
}

// New validates the configuration and returns the service.
func New(db *gorm.DB, store Store, rpID, rpName string, origins []string) (*Service, error) {
	if rpID == "" || len(origins) == 0 {
		return nil, errors.New("webauthn: rp id and origins are required")
	}
	if rpName == "" {
		rpName = "Mailez"
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                 rpID,
		RPDisplayName:        rpName,
		RPOrigins:            origins,
		EncodeUserIDAsString: true,
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn: %w", err)
	}
	return &Service{DB: db, Store: store, RPID: rpID, RPName: rpName, Origins: origins, wa: wa}, nil
}

// ErrUnavailable reports that the account has no passkeys (or does not
// exist); callers map it to a coarse 4xx so account existence is not
// disclosed beyond what the email-first flow already implies.
var ErrUnavailable = errors.New("webauthn: no credentials for account")

// waUser adapts a user row to the library's interface. The WebAuthn user ID
// must be stable and non-identifying: SHA-256 of the e-mail is both.
type waUser struct {
	email       string
	displayName string
	creds       []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte {
	h := sha256.Sum256([]byte(u.email))
	return h[:]
}

func (u *waUser) WebAuthnName() string        { return u.email }
func (u *waUser) WebAuthnDisplayName() string { return u.displayName }
func (u *waUser) WebAuthnIcon() string        { return "" }

func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Service) loadUser(email string) (*waUser, error) {
	var u models.User
	if err := s.DB.Where("email = ?", email).First(&u).Error; err != nil {
		return nil, ErrUnavailable
	}
	var rows []models.WebauthnCredential
	if err := s.DB.Where("user_email = ?", email).Find(&rows).Error; err != nil {
		return nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	for _, r := range rows {
		creds = append(creds, webauthn.Credential{
			ID:              []byte(r.CredentialID),
			PublicKey:       r.PublicKey,
			AttestationType: r.Attestation,
			Transport:       transportsFrom(r.Transports),
			Authenticator: webauthn.Authenticator{
				AAGUID:    []byte(r.AAGUID),
				SignCount: r.SignCount,
			},
		})
	}
	return &waUser{email: u.Email, displayName: u.DisplayedName, creds: creds}, nil
}

func transportsFrom(s string) []protocol.AuthenticatorTransport {
	if s == "" {
		return nil
	}
	var out []protocol.AuthenticatorTransport
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, protocol.AuthenticatorTransport(t))
		}
	}
	return out
}

func transportsTo(in []protocol.AuthenticatorTransport) string {
	parts := make([]string, 0, len(in))
	for _, t := range in {
		parts = append(parts, string(t))
	}
	return strings.Join(parts, ",")
}

func (s *Service) saveSession(kind, key string, sd *webauthn.SessionData) error {
	blob, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	return s.Store.Set(context.Background(), "mailez:webauthn:"+kind+":"+key, string(blob), challengeTTL)
}

func (s *Service) loadSession(kind, key string) (*webauthn.SessionData, error) {
	raw, ok, err := s.Store.Get(context.Background(), "mailez:webauthn:"+kind+":"+key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("webauthn: ceremony expired, start again")
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

// BeginRegistration creates a credential-creation ceremony for the account.
// The returned options go to the browser's navigator.credentials.create().
func (s *Service) BeginRegistration(email string) (any, error) {
	u, err := s.loadUser(email)
	if err != nil {
		return nil, err
	}
	if u.displayName == "" {
		u.displayName = u.email
	}
	options, sd, err := s.wa.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		return nil, err
	}
	if err := s.saveSession("reg", email, sd); err != nil {
		return nil, err
	}
	return options, nil
}

// FinishRegistration verifies the attestation response (the browser's
// PublicKeyCredential JSON body) and stores the credential under the given
// display name.
func (s *Service) FinishRegistration(email, name string, body []byte) (*models.WebauthnCredential, error) {
	sd, err := s.loadSession("reg", email)
	if err != nil {
		return nil, err
	}
	_ = s.Store.Delete(context.Background(), "mailez:webauthn:reg:"+email)
	u, err := s.loadUser(email)
	if err != nil {
		return nil, err
	}
	if u.displayName == "" {
		u.displayName = u.email
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return nil, err
	}
	cred, err := s.wa.CreateCredential(u, *sd, parsed)
	if err != nil {
		return nil, err
	}
	row := models.WebauthnCredential{
		UserEmail:    email,
		Name:         name,
		CredentialID: string(cred.ID),
		PublicKey:    cred.PublicKey,
		Attestation:  cred.AttestationType,
		Transports:   transportsTo(cred.Transport),
		AAGUID:       string(cred.Authenticator.AAGUID),
		SignCount:    cred.Authenticator.SignCount,
	}
	if err := s.DB.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// BeginLogin creates an assertion ceremony scoped to the account's existing
// credentials (e-mail known flow). Passkeys remain single-account.
func (s *Service) BeginLogin(email string) (any, error) {
	u, err := s.loadUser(email)
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(u.creds) == 0 {
		return nil, ErrUnavailable
	}
	options, sd, err := s.wa.BeginLogin(u)
	if err != nil {
		return nil, err
	}
	if err := s.saveSession("login", email, sd); err != nil {
		return nil, err
	}
	return options, nil
}

// FinishLogin verifies the assertion (the browser's PublicKeyCredential
// JSON body), bumps the sign count and reports the credential used. A nil
// error means the user is authenticated.
func (s *Service) FinishLogin(email string, body []byte) error {
	sd, err := s.loadSession("login", email)
	if err != nil {
		return err
	}
	_ = s.Store.Delete(context.Background(), "mailez:webauthn:login:"+email)
	u, err := s.loadUser(email)
	if err != nil {
		return err
	}
	var car protocol.CredentialAssertionResponse
	if err := json.Unmarshal(body, &car); err != nil {
		return err
	}
	parsed, err := car.Parse()
	if err != nil {
		return err
	}
	cred, err := s.wa.ValidateLogin(u, *sd, parsed)
	if err != nil {
		return err
	}
	// Persist the authenticator's sign count (clone detection) and usage.
	now := time.Now()
	return s.DB.Model(&models.WebauthnCredential{}).
		Where("user_email = ? AND credential_id = ?", email, string(cred.ID)).
		Updates(map[string]any{"sign_count": cred.Authenticator.SignCount, "last_used_at": &now}).Error
}

// List returns the account's registered credentials (PublicKey is omitted
// from JSON by the model tag).
func (s *Service) List(email string) ([]models.WebauthnCredential, error) {
	var rows []models.WebauthnCredential
	err := s.DB.Where("user_email = ?", email).
		Order("created_at asc").
		Find(&rows).Error
	return rows, err
}

// Delete removes one credential; only the owner may delete it.
func (s *Service) Delete(email string, id uint) error {
	return s.DB.Where("user_email = ? AND id = ?", email, id).
		Delete(&models.WebauthnCredential{}).Error
}
