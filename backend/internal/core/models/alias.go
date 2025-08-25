package models

// Alias is an email address that redirects to some destination.
type Alias struct {
	Base
	Email       string `gorm:"primaryKey;size:255;not null" json:"email"`
	Localpart   string `gorm:"size:80;not null" json:"localpart"`
	DomainName  string `gorm:"size:80;not null;index:idx_aliases_domain_disabled,priority:1" json:"domain_name"`
	Wildcard    bool   `gorm:"not null;default:false" json:"wildcard"`
	Destination string `gorm:"size:4096;not null" json:"destination"`
	Disabled    bool   `gorm:"not null;default:false;index:idx_aliases_domain_disabled,priority:2" json:"disabled"`

	// Anonymous Email Service metadata
	Hostname   string `gorm:"size:255" json:"hostname"`
	OwnerEmail string `gorm:"size:255;index:idx_aliases_owner_email" json:"owner_email"`
}

// Destinations returns the parsed destination list.
func (a *Alias) Destinations() []string {
	return splitCSV(a.Destination)
}
