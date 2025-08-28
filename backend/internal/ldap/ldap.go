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
	"sort"
	"strings"
	"sync"
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
	// CheckCapacity, when set, is consulted before auto-provisioning a
	// mailbox so directory sync cannot bypass the license's mailbox cap.
	CheckCapacity func(db *gorm.DB) error

	// groupCache memoizes delivery-time member expansion for a short TTL so
	// directory changes land within a minute without hammering LDAP on every
	// message to a group mailbox.
	groupCache sync.Map // groupEmail -> cachedGroup
}

type cachedGroup struct {
	members []string
	expires time.Time
}

// groupCacheTTL bounds how stale a delivery-time expansion may be.
const groupCacheTTL = 60 * time.Second

// maxGroupDepth guards against pathological nesting in the directory.
const maxGroupDepth = 12

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

// findUserDN locates the directory entry for an address using MailAttr first,
// then the AD userPrincipalName attribute, then UIDAttr (local part) so
// sAMAccountName-only directories authenticate as localpart@domain.
func findUserDN(conn *ldap.Conn, cfg *models.LdapConfig, email string) (string, error) {
	local := email
	if i := strings.LastIndex(email, "@"); i >= 0 {
		local = email[:i]
	}
	filters := []string{
		fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.MailAttr, ldap.EscapeFilter(email)),
	}
	if cfg.UpnAttr != "" {
		filters = append(filters, fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.UpnAttr, ldap.EscapeFilter(email)))
	}
	if cfg.UIDAttr != "" {
		filters = append(filters, fmt.Sprintf("(&%s(%s=%s))", cfg.UserFilter, cfg.UIDAttr, ldap.EscapeFilter(local)))
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

// entryEmail derives the mailbox address of a directory entry: the mail
// attribute, then the AD userPrincipalName, then uid@EmailDomain for
// sAMAccountName-only directories.
func entryEmail(e *ldap.Entry, cfg *models.LdapConfig) string {
	if v := firstValue(e, cfg.MailAttr); v != "" {
		return v
	}
	if cfg.UpnAttr != "" {
		if v := firstValue(e, cfg.UpnAttr); v != "" && strings.Contains(v, "@") {
			return v
		}
	}
	if cfg.EmailDomain != "" && cfg.UIDAttr != "" {
		if v := firstValue(e, cfg.UIDAttr); v != "" {
			return v + "@" + cfg.EmailDomain
		}
	}
	return ""
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
	if s.CheckCapacity != nil {
		if err := s.CheckCapacity(s.DB); err != nil {
			return fmt.Errorf("ldap: %w", err)
		}
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
	now := time.Now()
	u := models.User{
		Email:             email,
		Localpart:         local,
		DomainName:        domain,
		Password:          hash,
		Enabled:           true,
		QuotaBytes:        d.MaxQuotaBytes,
		LdapManaged:       true,
		PasswordChangedAt: &now,
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
			"dn", cfg.MailAttr, cfg.UIDAttr, cfg.UpnAttr, cfg.NameAttr,
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
		email := strings.ToLower(strings.TrimSpace(entryEmail(e, cfg)))
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
			"dn", cfg.MailAttr, cfg.UIDAttr, cfg.UpnAttr, cfg.NameAttr, cfg.DeptAttr, cfg.TitleAttr, cfg.PhoneAttr,
		},
		TimeLimit: 30,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("ldap search: %w", err)
	}
	for _, e := range res.Entries {
		email := entryEmail(e, cfg)
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

// SyncGroups reconciles distribution-list aliases with directory groups:
// each group becomes a local Alias (LdapGroup=true) whose destination is the
// member mailboxes. Manually created aliases are never overwritten, and
// groups that disappeared are disabled rather than deleted.
func (s *Service) SyncGroups(ctx context.Context) (created, updated, disabled int, err error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	if !cfg.SyncGroups || cfg.GroupFilter == "" {
		return 0, 0, 0, nil
	}
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return 0, 0, 0, err
	}
	defer conn.Close()
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN: cfg.BaseDN,
		Scope:  ldap.ScopeWholeSubtree,
		Filter: cfg.GroupFilter,
		Attributes: []string{
			"dn", cfg.GroupNameAttr, cfg.GroupMailAttr, cfg.GroupMemberAttr,
		},
		TimeLimit: 30,
	})
	if err != nil {
		return 0, 0, 0, fmt.Errorf("ldap group search: %w", err)
	}

	// uid -> mailbox map from local accounts (the account sync provisions
	// directory users first, so member DNs resolve to real mailboxes).
	var users []models.User
	if err := s.DB.WithContext(ctx).Find(&users).Error; err != nil {
		return 0, 0, 0, err
	}
	byLocal := map[string]string{}
	for i := range users {
		byLocal[strings.ToLower(users[i].Localpart)] = users[i].Email
	}

	groupEmails := map[string]bool{}
	for _, g := range res.Entries {
		groupEmail := strings.ToLower(strings.TrimSpace(firstValue(g, cfg.GroupMailAttr)))
		if groupEmail == "" {
			cn := firstValue(g, cfg.GroupNameAttr)
			if cn == "" || cfg.EmailDomain == "" {
				continue
			}
			groupEmail = strings.ToLower(cn) + "@" + cfg.EmailDomain
		}
		groupEmails[groupEmail] = true

		members := map[string]bool{}
		for _, dn := range g.GetAttributeValues(cfg.GroupMemberAttr) {
			if email := strings.TrimSpace(dn); strings.Contains(email, "@") && !strings.Contains(email, "=") {
				members[strings.ToLower(email)] = true
				continue
			}
			uid := dnUID(dn)
			if uid == "" {
				continue
			}
			if email, ok := byLocal[strings.ToLower(uid)]; ok {
				members[email] = true
			}
		}
		if len(members) == 0 {
			continue // skip empty groups
		}
		dest := make([]string, 0, len(members))
		for m := range members {
			dest = append(dest, m)
		}
		sort.Strings(dest)

		local, domain, ok := strings.Cut(groupEmail, "@")
		if !ok || local == "" || domain == "" {
			continue
		}
		alias := models.Alias{
			Email:       groupEmail,
			Localpart:   local,
			DomainName:  domain,
			Destination: strings.Join(dest, ","),
			Disabled:    false,
			LdapGroup:   true,
		}
		var cur models.Alias
		found := s.DB.WithContext(ctx).First(&cur, "email = ?", groupEmail).Error == nil
		if found && !cur.LdapGroup {
			continue // manual alias wins
		}
		if found {
			if cur.Destination == alias.Destination && !cur.Disabled {
				continue
			}
			alias.Base = cur.Base
			if err := s.DB.WithContext(ctx).Save(&alias).Error; err != nil {
				return created, updated, disabled, err
			}
			updated++
		} else {
			if err := s.DB.WithContext(ctx).Create(&alias).Error; err != nil {
				return created, updated, disabled, err
			}
			created++
		}
	}

	// Disable synced aliases whose group vanished (never delete, and never
	// touch manually created aliases).
	var synced []models.Alias
	if err := s.DB.WithContext(ctx).Where("ldap_group = ?", true).Find(&synced).Error; err != nil {
		return created, updated, disabled, err
	}
	for i := range synced {
		if groupEmails[strings.ToLower(synced[i].Email)] || synced[i].Disabled {
			continue
		}
		if err := s.DB.WithContext(ctx).Model(&synced[i]).Update("disabled", true).Error; err != nil {
			return created, updated, disabled, err
		}
		disabled++
	}
	return created, updated, disabled, nil
}

// ResolveGroupMembers returns the effective member mailboxes of a group
// mailbox at delivery time: the directory is queried live (with a short
// cache), nested groups are expanded recursively with loop protection, and
// user DNs resolve through local accounts or their directory entry. An error
// is returned when the directory integration is off so callers can fall back
// to the last synced member list.
func (s *Service) ResolveGroupMembers(ctx context.Context, groupEmail string) ([]string, error) {
	cfg, err := s.config(ctx)
	if err != nil || !cfg.SyncGroups {
		return nil, errors.New("ldap: group sync not enabled")
	}
	key := strings.ToLower(strings.TrimSpace(groupEmail))
	if key == "" {
		return nil, errors.New("ldap: empty group email")
	}
	if v, ok := s.groupCache.Load(key); ok {
		if c := v.(cachedGroup); time.Now().Before(c.expires) {
			return append([]string(nil), c.members...), nil
		}
	}

	local, _, ok := strings.Cut(key, "@")
	if !ok || local == "" {
		return nil, errors.New("ldap: invalid group email")
	}
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Locate the group entry: by its mail attribute or by name@email_domain.
	groupFilter := fmt.Sprintf("(&%s(|(%s=%s)(%s=%s)))",
		cfg.GroupFilter,
		cfg.GroupMailAttr, ldap.EscapeFilter(key),
		cfg.GroupNameAttr, ldap.EscapeFilter(local))
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     cfg.BaseDN,
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     groupFilter,
		Attributes: []string{"dn", cfg.GroupMemberAttr},
		TimeLimit:  10,
	})
	if err != nil {
		return nil, fmt.Errorf("ldap group lookup: %w", err)
	}
	if len(res.Entries) == 0 {
		return nil, errors.New("ldap: group not found in directory")
	}

	// uid -> mailbox from local accounts (delivery targets must be real
	// mailboxes; the account sync provisions directory users).
	var users []models.User
	if err := s.DB.WithContext(ctx).Find(&users).Error; err != nil {
		return nil, err
	}
	byLocal := map[string]string{}
	for i := range users {
		byLocal[strings.ToLower(users[i].Localpart)] = users[i].Email
	}

	out := map[string]bool{}
	visited := map[string]bool{}
	if err := s.expandGroup(ctx, conn, cfg, res.Entries[0].DN, byLocal, visited, out, 0); err != nil {
		return nil, err
	}
	members := make([]string, 0, len(out))
	for m := range out {
		members = append(members, m)
	}
	sort.Strings(members)
	s.groupCache.Store(key, cachedGroup{members: members, expires: time.Now().Add(groupCacheTTL)})
	return members, nil
}

// expandGroup recursively resolves a group DN's members. Member DNs that
// match a local account become that mailbox; other user DNs are looked up in
// the directory for their mail/UPN/uid-derived address; member groups are
// expanded recursively with a visited set and depth cap.
func (s *Service) expandGroup(ctx context.Context, conn *ldap.Conn, cfg *models.LdapConfig,
	groupDN string, byLocal map[string]string, visited map[string]bool, out map[string]bool, depth int) error {
	if depth > maxGroupDepth || visited[groupDN] {
		return nil
	}
	visited[groupDN] = true
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     groupDN,
		Scope:      ldap.ScopeBaseObject,
		Filter:     "(objectClass=*)",
		Attributes: []string{"dn", cfg.GroupMemberAttr},
		TimeLimit:  10,
	})
	if err != nil || len(res.Entries) == 0 {
		return nil // missing group: treat as empty, don't fail delivery
	}
	for _, memberDN := range res.Entries[0].GetAttributeValues(cfg.GroupMemberAttr) {
		uid := dnUID(memberDN)
		if uid != "" {
			if email, ok := byLocal[strings.ToLower(uid)]; ok {
				out[email] = true
				continue
			}
		}
		// A member may be a nested group (recurse) or a user entry without a
		// local account yet (resolve its directory mailbox).
		entry, err := s.entryByDN(ctx, conn, cfg, memberDN)
		if err != nil || entry == nil {
			continue
		}
		// Nested group: recurse (its own mail attribute, if any, is the group
		// mailbox address, not a delivery target).
		if isGroupEntry(entry) {
			if err := s.expandGroup(ctx, conn, cfg, memberDN, byLocal, visited, out, depth+1); err != nil {
				return err
			}
			continue
		}
		if email := entryEmail(entry, cfg); email != "" {
			out[strings.ToLower(email)] = true
			continue
		}
	}
	return nil
}

// isGroupEntry reports whether a directory entry is a group object class.
func isGroupEntry(e *ldap.Entry) bool {
	for _, attr := range e.Attributes {
		if !strings.EqualFold(attr.Name, "objectclass") {
			continue
		}
		for _, oc := range attr.Values {
			switch strings.ToLower(oc) {
			case "groupofnames", "groupofuniquenames", "group", "distributionlist", "mailgroup":
				return true
			}
		}
	}
	return false
}

// entryByDN fetches a directory entry by its DN.
func (s *Service) entryByDN(ctx context.Context, conn *ldap.Conn, cfg *models.LdapConfig, dn string) (*ldap.Entry, error) {
	res, err := conn.Search(&ldap.SearchRequest{
		BaseDN: dn,
		Scope:  ldap.ScopeBaseObject,
		Filter: "(objectClass=*)",
		Attributes: []string{
			"dn", cfg.MailAttr, cfg.UIDAttr, cfg.UpnAttr, "objectClass",
		},
		TimeLimit: 10,
	})
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		return nil, nil
	}
	return res.Entries[0], nil
}

// dnUID extracts the first RDN value of a distinguished name
// ("uid=alice,ou=people,dc=example,dc=com" -> "alice").
func dnUID(dn string) string {
	if i := strings.Index(dn, "="); i > 0 {
		rest := dn[i+1:]
		if j := strings.Index(rest, ","); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return ""
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
