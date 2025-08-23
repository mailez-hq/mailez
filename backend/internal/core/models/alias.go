package models

// Alias is an email address that redirects to some destination.
type Alias struct {
	Base
	Email       string `gorm:"primaryKey;size:255;not null" json:"email"`
	Localpart   string `gorm:"size:80;not null" json:"localpart"`
	DomainName  string `gorm:"size:80;not null" json:"domain_name"`
	Wildcard    bool   `gorm:"not null;default:false" json:"wildcard"`
	Destination string `gorm:"size:4096;not null" json:"destination"`

	// Anonymous Email Service metadata
	Hostname   string `gorm:"size:255" json:"hostname"`
	OwnerEmail string `gorm:"size:255" json:"owner_email"`
	Disabled   bool   `gorm:"not null;default:false" json:"disabled"`
}

// Destinations returns the parsed destination list.
func (a *Alias) Destinations() []string {
	return splitCSV(a.Destination)
}
