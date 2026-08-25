package models

// Domain is a DNS domain that has mail addresses associated to it.
type Domain struct {
	Base
	Name            string `gorm:"primaryKey;size:80;not null" json:"name"`
	MaxUsers        int    `gorm:"not null;default:-1" json:"max_users"`
	MaxAliases      int    `gorm:"not null;default:-1" json:"max_aliases"`
	MaxQuotaBytes   int64  `gorm:"not null;default:0" json:"max_quota_bytes"`
	SignupEnabled   bool   `gorm:"not null;default:false" json:"signup_enabled"`
	AnonmailEnabled bool   `gorm:"not null;default:false" json:"anonmail_enabled"`
	DkimKey         string `gorm:"type:text" json:"-"`

	Users        []User        `gorm:"foreignKey:DomainName;references:Name" json:"-"`
	Aliases      []Alias       `gorm:"foreignKey:DomainName;references:Name" json:"-"`
	Alternatives []Alternative `gorm:"foreignKey:DomainName;references:Name" json:"-"`
	Managers     []User        `gorm:"many2many:manager" json:"-"`
}

// Alternative is an alternative name for a served domain.
type Alternative struct {
	Base
	Name       string `gorm:"primaryKey;size:80;not null" json:"name"`
	DomainName string `gorm:"size:80;not null;index:idx_alternatives_domain_name" json:"domain_name"`
}

// Relay is a relayed mail domain, optionally via a specific SMTP host.
type Relay struct {
	Base
	Name string `gorm:"primaryKey;size:80;not null" json:"name"`
	SMTP string `gorm:"size:80" json:"smtp"`
}

// DomainAccess grants a user Anonymous Email Service access to a domain.
type DomainAccess struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	DomainName string `gorm:"size:80;not null;uniqueIndex:uq_domain_access_user" json:"domain_name"`
	UserEmail  string `gorm:"size:255;uniqueIndex:uq_domain_access_user" json:"user_email"`
}
