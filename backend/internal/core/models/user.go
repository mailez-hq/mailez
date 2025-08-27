package models

import "time"

// User is an email address that has a password to access a mailbox.
type User struct {
	Base
	Email          string `gorm:"primaryKey;size:255;not null" json:"email"`
	Localpart      string `gorm:"size:80;not null" json:"localpart"`
	DomainName     string `gorm:"size:80;not null;index:idx_users_domain_name" json:"domain_name"`
	Password       string `gorm:"size:255;not null" json:"-"`
	QuotaBytes     int64  `gorm:"not null;default:1000000000" json:"quota_bytes"`
	QuotaBytesUsed int64  `gorm:"not null;default:0" json:"quota_bytes_used"`
	GlobalAdmin    bool   `gorm:"not null;default:false" json:"global_admin"`
	Enabled        bool   `gorm:"not null;default:true;index:idx_users_enabled" json:"enabled"`
	EnableImap     bool   `gorm:"not null;default:true" json:"enable_imap"`
	EnablePop      bool   `gorm:"not null;default:true" json:"enable_pop"`
	AllowSpoofing  bool   `gorm:"not null;default:false" json:"allow_spoofing"`
	// LdapManaged marks accounts provisioned from the AD/LDAP directory: the
	// lifecycle sync may enable/disable them as directory membership changes
	// (manually created accounts are never touched).
	LdapManaged        bool       `gorm:"not null;default:false" json:"ldap_managed"`
	ForwardEnabled     bool       `gorm:"not null;default:false" json:"forward_enabled"`
	ForwardDestination string     `gorm:"size:4096" json:"forward_destination"`
	ForwardKeep        bool       `gorm:"not null;default:true" json:"forward_keep"`
	ReplyEnabled       bool       `gorm:"not null;default:false" json:"reply_enabled"`
	ReplySubject       string     `gorm:"size:255" json:"reply_subject"`
	ReplyBody          string     `gorm:"type:text" json:"reply_body"`
	ReplyStartdate     *time.Time `gorm:"type:date" json:"reply_startdate"`
	ReplyEnddate       *time.Time `gorm:"type:date" json:"reply_enddate"`
	DisplayedName      string     `gorm:"size:160;not null;default:''" json:"displayed_name"`
	SpamEnabled        bool       `gorm:"not null;default:true" json:"spam_enabled"`
	SpamMarkAsRead     bool       `gorm:"not null;default:true" json:"spam_mark_as_read"`
	SpamThreshold      int        `gorm:"not null;default:80" json:"spam_threshold"`
	ChangePwNextLogin  bool       `gorm:"not null;default:false" json:"change_pw_next_login"`
	TOTPSecret         string     `gorm:"size:64" json:"-"`
	TOTPEnabled        bool       `gorm:"not null;default:false" json:"totp_enabled"`
	Whitelist          string     `gorm:"type:text" json:"whitelist"`
	Blacklist          string     `gorm:"type:text" json:"blacklist"`
	Signature          string     `gorm:"type:text" json:"signature"`
	PGPPublicKey       string     `gorm:"type:text" json:"pgp_public_key"`
	PGPPrivateKey      string     `gorm:"type:text" json:"-"` // armored, encrypted at rest with SECRET_KEY
	PGPFingerprint     string     `gorm:"size:64" json:"pgp_fingerprint"`
	SmimeCert          string     `gorm:"type:text" json:"smime_cert"` // own X.509 certificate (PEM)
	SmimePrivateKey    string     `gorm:"type:text" json:"-"`          // PEM, encrypted at rest with SECRET_KEY
	SmimeFingerprint   string     `gorm:"size:64" json:"smime_fingerprint"`
	SmimeEmail         string     `gorm:"size:255" json:"smime_email"`
	SmimeNotAfter      *time.Time `gorm:"type:date" json:"smime_not_after"`
	// KnownIPs is the rolling list of IPs this account has logged in from;
	// a login from a new IP triggers the security alert email.
	KnownIPs string `gorm:"type:text" json:"-"`

	Tokens  []Token `gorm:"foreignKey:UserEmail" json:"-"`
	Fetches []Fetch `gorm:"foreignKey:UserEmail" json:"-"`
}

// Destination returns the comma-separated delivery destinations, honouring
// forwarding settings.
func (u *User) Destination() string {
	if !u.ForwardEnabled {
		return u.Email
	}
	dest := splitCSV(u.ForwardDestination)
	if u.ForwardKeep {
		dest = append(dest, u.Email)
	}
	return joinCSV(dest)
}

// ReplyActive reports whether the auto-reply is currently active.
func (u *User) ReplyActive() bool {
	if !u.ReplyEnabled {
		return false
	}
	if u.ReplyStartdate == nil || u.ReplyEnddate == nil {
		return false
	}
	now := time.Now()
	return !now.Before(*u.ReplyStartdate) && !now.After(*u.ReplyEnddate)
}
