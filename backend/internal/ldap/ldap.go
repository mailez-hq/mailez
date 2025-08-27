// Package ldap implements directory integration with AD/LDAP: password
// authentication against the directory (with optional local-user
// auto-provisioning) and a read-only organization address book synced from
// the directory. The connection is configured through the admin console and
// stored encrypted in the LdapConfig row.
package ldap

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/password"
)

// Service talks to the configured directory.
type Service struct {
	DB        *gorm.DB
	SecretKey string
}

// New returns an LDAP service bound to the database.
func New(db *gorm.DB, secretKey string) *Service {
	return &Service{DB: db, SecretKey: secretKey}
}

// Enabled reports whether directory integration is switched on.
func (s *Service) Enabled(ctx context.Context) bool {
	cfg, err := s.config(ctx)
	return err == nil && cfg.Enabled
}

func (s *Service) config(ctx context.Context) (*models.LdapConfig, error) {
	var cfg models.LdapConfig
	if err := s.DB.WithContext(ctx).First(&cfg).Error; err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.Host == "" || cfg.BaseDN == "" {
		return nil, errors.New("ldap: not enabled")
	}
	return &cfg, nil
}

// bindPassword decrypts the stored bind credential (empty bind = anonymous).
func (s *Service) bindPassword(ctx context.Context, cfg *models.LdapConfig) (string, error) {
	if cfg.BindPasswordEnc == "" {
		return "", nil
	}
	return crypto.Decrypt(s.SecretKey, cfg.BindPasswordEnc)
}

// dial connects to the directory, upgrades transport per Security and binds
// with the service account when configured.
func (s *Service) dial(ctx context.Context, cfg *models.LdapConfig) (*ldap.Conn, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	url := "ldap://" + addr
	tlsCfg := &tls.Config{InsecureSkipVerify: true, ServerName: cfg.Host}
	var conn *ldap.Conn
	var err error
	switch cfg.Security {
	case "tls":
		url = "ldaps://" + addr
		conn, err = ldap.DialURL(url, ldap.DialWithTLSConfig(tlsCfg), ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}))
	default:
		conn, err = ldap.DialURL(url, ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}))
	}
	if err != nil {
		return nil, fmt.Errorf("ldap dial %s: %w", addr, err)
	}
	conn.SetTimeout(10 * time.Second)
	if cfg.Security == "starttls" {
		if err := conn.StartTLS(tlsCfg); err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldap starttls: %w", err)
		}
	}
	if cfg.BindDN != "" {
		pw, err := s.bindPassword(ctx, cfg)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldap bind password: %w", err)
		}
		if err := conn.Bind(cfg.BindDN, pw); err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldap bind: %w", err)
		}
	}
	return conn, nil
}

// findUserDN locates the directory entry for an address using MailAttr first
// and UIDAttr (local part) as a fallback.
func findUserDN(conn *ldap.Conn, cfg *models.LdapConfig, email string) (string, error) {
	local := email
	if i := strings.LastIndex(email, "@"); i >= 0 {
		local = email[:i]
	}
	filters := []string{
		fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.MailAttr, ldap.EscapeFilter(email)),
		fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.UIDAttr, ldap.EscapeFilter(local)),
	}
	for _, f := range filters {
		res, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     cfg.BaseDN,
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     f,
			Attributes: []string{"dn"},
			TimeLimit:  10,
		})
		if err != nil {
			return "", err
		}
		if len(res.Entries) > 0 {
			return res.Entries[0].DN, nil
		}
	}
	return "", errors.New("ldap: user not found in directory")
}

// Authenticate validates email/password against the directory. It returns
// false (not an error) when the directory is disabled or the user is absent,
// so local password auth keeps working as the fallback.
func (s *Service) Authenticate(ctx context.Context, email, password string) (bool, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return false, nil
	}
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	dn, err := findUserDN(conn, cfg, email)
	if err != nil {
		return false, nil // absent from directory: let local auth decide
	}
	if err := conn.Bind(dn, password); err != nil {
		return false, nil // wrong password
	}
	return true, nil
}

// EnsureLocalUser provisions a local mailbox account for a directory user
// after a successful bind. Existing accounts pass through untouched; the
// account gets a random local password so the directory remains the only
// credential source.
func (s *Service) EnsureLocalUser(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	var existing models.User
	if err := s.DB.WithContext(ctx).First(&existing, "email = ?", email).Error; err == nil {
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	if !cfg.AutoCreate {
		return errors.New("ldap: user not provisioned locally")
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return errors.New("ldap: invalid email")
	}
	var d models.Domain
	if err := s.DB.WithContext(ctx).First(&d, "name = ?", domain).Error; err != nil {
		return fmt.Errorf("ldap: domain %q is not served locally", domain)
	}
	randPw, err := randomSecret()
	if err != nil {
		return err
	}
	hash, err := password.Hash(randPw)
	if err != nil {
		return err
	}
	u := models.User{
		Email:       email,
		Localpart:   local,
		DomainName:  domain,
		Password:    hash,
		Enabled:     true,
		QuotaBytes:  d.MaxQuotaBytes,
		LdapManaged: true,
	}
	if u.QuotaBytes <= 0 {
		u.QuotaBytes = 1_000_000_000
	}
	return s.DB.WithContext(ctx).Create(&u).Error
}

// SyncAccounts reconciles local LDAP-managed accounts with the directory:
//   - directory users without a local account are provisioned (AutoCreate);
//   - local LDAP-managed accounts missing from the directory are disabled
//     (leavers) — manually created accounts are never touched;
//   - display names follow the directory.
//
// The disable step is skipped when the directory returns no users at all,
// which usually means a filter/base-DN configuration problem rather than an
// empty organization, so a config typo cannot mass-disable the mailbox.
func (s *Service) SyncAccounts(ctx context.Context) (created, disabled int, err error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return 0, 0, err
	}
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN: cfg.BaseDN,
		Scope:  ldap.ScopeWholeSubtree,
		Filter: cfg.UserFilter,
		Attributes: []string{
			"dn", cfg.MailAttr, cfg.NameAttr,
		},
		TimeLimit: 30,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("ldap search: %w", err)
	}
	if len(res.Entries) == 0 {
		return 0, 0, nil // empty directory: skip provisioning and disabling
	}

	dirEmails := map[string]string{} // email -> display name
	for _, e := range res.Entries {
		email := strings.ToLower(strings.TrimSpace(firstValue(e, cfg.MailAttr)))
		if email == "" {
			continue
		}
		dirEmails[email] = firstValue(e, cfg.NameAttr)
	}

	var managed []models.User
	if err := s.DB.WithContext(ctx).Where("ldap_managed = ?", true).Find(&managed).Error; err != nil {
		return 0, 0, err
	}
	localByEmail := map[string]*models.User{}
	for i := range managed {
		localByEmail[strings.ToLower(managed[i].Email)] = &managed[i]
	}

	for email, name := range dirEmails {
		if cur, ok := localByEmail[email]; ok {
			// Name follows the directory; enabled state is left alone so an
			// admin's manual disable is never overridden by a re-appearing
			// directory entry.
			if name != "" && name != cur.DisplayedName {
				cur.DisplayedName = name
				if err := s.DB.WithContext(ctx).Model(cur).Update("displayed_name", name).Error; err != nil {
					return created, disabled, err
				}
			}
			continue
		}
		if !cfg.AutoCreate {
			continue
		}
		if err := s.EnsureLocalUser(ctx, email); err != nil {
			return created, disabled, err
		}
		created++
	}

	for email, cur := range localByEmail {
		if _, ok := dirEmails[email]; ok {
			continue
		}
		if !cur.Enabled {
			continue
		}
		if err := s.DB.WithContext(ctx).Model(cur).Update("enabled", false).Error; err != nil {
			return created, disabled, err
		}
		disabled++
	}
	return created, disabled, nil
}

// SyncContacts refreshes the read-only organization address book from the
// directory: rows are inserted/updated by email; nothing is deleted so a
// temporary directory outage cannot wipe the org book.
func (s *Service) SyncContacts(ctx context.Context) (added, updated int, err error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return 0, 0, err
	}
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN: cfg.BaseDN,
		Scope:  ldap.ScopeWholeSubtree,
		Filter: cfg.UserFilter,
		Attributes: []string{
			"dn", cfg.MailAttr, cfg.NameAttr, cfg.DeptAttr, cfg.TitleAttr, cfg.PhoneAttr,
		},
		TimeLimit: 30,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("ldap search: %w", err)
	}
	for _, e := range res.Entries {
		email := firstValue(e, cfg.MailAttr)
		if email == "" {
			continue
		}
		email = strings.ToLower(strings.TrimSpace(email))
		row := models.OrgContact{
			Email:      email,
			Name:       firstValue(e, cfg.NameAttr),
			Department: firstValue(e, cfg.DeptAttr),
			Title:      firstValue(e, cfg.TitleAttr),
			Phone:      firstValue(e, cfg.PhoneAttr),
			LdapDN:     e.DN,
		}
		var cur models.OrgContact
		found := s.DB.WithContext(ctx).Where("email = ?", email).First(&cur).Error == nil
		if found {
			if cur.Name == row.Name && cur.Department == row.Department &&
				cur.Title == row.Title && cur.Phone == row.Phone {
				continue
			}
			row.ID = cur.ID
			if err := s.DB.WithContext(ctx).Save(&row).Error; err != nil {
				return added, updated, err
			}
			updated++
		} else {
			if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
				return added, updated, err
			}
			added++
		}
	}
	return added, updated, nil
}

// TestConnection validates a candidate configuration (bind + a single user
// search) before the admin saves it.
func (s *Service) TestConnection(ctx context.Context, in models.LdapConfig, bindPassword string) error {
	cfg := in
	if bindPassword != "" {
		enc, err := crypto.Encrypt(s.SecretKey, bindPassword)
		if err != nil {
			return err
		}
		cfg.BindPasswordEnc = enc
	}
	conn, err := s.dial(ctx, &cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Search(&ldap.SearchRequest{
		BaseDN:     cfg.BaseDN,
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     cfg.UserFilter,
		Attributes: []string{"dn"},
		TimeLimit:  10,
	})
	if err != nil {
		return fmt.Errorf("ldap search: %w", err)
	}
	return nil
}

func firstValue(e *ldap.Entry, attr string) string {
	if attr == "" {
		return ""
	}
	if v := e.GetAttributeValue(attr); v != "" {
		return v
	}
	return ""
}

func randomSecret() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
