package models

import "time"

// LdapConfig holds the directory integration settings for the whole
// deployment (single row, id = 1). The bind password is encrypted at rest
// with SECRET_KEY like fetch/account passwords.
type LdapConfig struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	Enabled         bool   `gorm:"not null;default:false" json:"enabled"`
	Host            string `gorm:"size:255;not null" json:"host"`
	Port            int    `gorm:"not null;default:389" json:"port"`
	Security        string `gorm:"size:16;not null;default:none" json:"security"` // none | starttls | tls
	BaseDN          string `gorm:"size:512;not null" json:"base_dn"`
	BindDN          string `gorm:"size:512" json:"bind_dn"`
	BindPasswordEnc string `gorm:"type:text" json:"-"`
	// No DB-level default: the filter contains parentheses, which SQLite
	// rejects as a non-constant default; the admin API supplies it.
	UserFilter string `gorm:"size:512;not null" json:"user_filter"`
	MailAttr   string `gorm:"size:64;not null;default:mail" json:"mail_attr"`
	UIDAttr    string `gorm:"size:64;not null;default:uid" json:"uid_attr"`
	// UpnAttr is the Active Directory userPrincipalName attribute; the mail
	// fallback when the mail attribute is empty on AD entries.
	UpnAttr string `gorm:"size:64;not null;default:userPrincipalName" json:"upn_attr"`
	// EmailDomain derives user mailboxes as uid@EmailDomain when neither
	// mail nor UPN carries an address (sAMAccountName-only directories).
	EmailDomain string `gorm:"size:255" json:"email_domain"`
	NameAttr    string `gorm:"size:64;not null;default:displayName" json:"name_attr"`
	DeptAttr    string `gorm:"size:64;default:department" json:"dept_attr"`
	TitleAttr   string `gorm:"size:64;default:title" json:"title_attr"`
	PhoneAttr   string `gorm:"size:64;default:telephoneNumber" json:"phone_attr"`
	AutoCreate  bool   `gorm:"not null;default:true" json:"auto_create"`
	// Group sync: distribution lists backed by directory groups.
	SyncGroups      bool      `gorm:"not null;default:false" json:"sync_groups"`
	GroupFilter     string    `gorm:"size:512" json:"group_filter"`
	GroupNameAttr   string    `gorm:"size:64;default:cn" json:"group_name_attr"`
	GroupMailAttr   string    `gorm:"size:64;default:mail" json:"group_mail_attr"`
	GroupMemberAttr string    `gorm:"size:64;default:member" json:"group_member_attr"`
	SyncMinutes     int       `gorm:"not null;default:60" json:"sync_minutes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// OrgContact is a read-only organization directory entry synced from LDAP
// (shared by every mailbox user, unlike per-user Contact rows).
type OrgContact struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Email      string    `gorm:"size:255;not null;uniqueIndex" json:"email"`
	Name       string    `gorm:"size:255;default:''" json:"name"`
	Department string    `gorm:"size:255;default:''" json:"department"`
	Title      string    `gorm:"size:255;default:''" json:"title"`
	Phone      string    `gorm:"size:64;default:''" json:"phone"`
	LdapDN     string    `gorm:"size:1024;default:''" json:"-"`
	UpdatedAt  time.Time `json:"updated_at"`
}
