package models

import (
  "encoding/json"
  "strings"
)

// AliasMember is one member of a distribution group (通讯组). Members can be
// local mailboxes or external addresses.
type AliasMember struct {
  Email string `json:"email"`
  Name  string `json:"name,omitempty"`
}

// Alias is an email address that redirects to some destination.
type Alias struct {
  Base
  Email       string `gorm:"primaryKey;size:255;not null" json:"email"`
  Localpart   string `gorm:"size:80;not null" json:"localpart"`
  DomainName  string `gorm:"size:80;not null;index:idx_aliases_domain_disabled,priority:1" json:"domain_name"`
  Wildcard    bool   `gorm:"not null;default:false" json:"wildcard"`
  Destination string `gorm:"size:4096;not null" json:"destination"`
  // Name is the display name of a distribution group (通讯组).
  Name string `gorm:"size:255;default:''" json:"name"`
  // Members stores the JSON-encoded distribution-group member list; the JSON
  // API exposes the parsed array via MarshalJSON.
  Members  string `gorm:"type:text" json:"-"`
  Disabled bool   `gorm:"not null;default:false;index:idx_aliases_domain_disabled,priority:2" json:"disabled"`

  // Anonymous Email Service metadata
  Hostname   string `gorm:"size:255" json:"hostname"`
  OwnerEmail string `gorm:"size:255;index:idx_aliases_owner_email" json:"owner_email"`
  // LdapGroup marks distribution-list aliases synced from directory groups;
  // the group sync owns them and never touches manually created aliases.
  LdapGroup bool `gorm:"not null;default:false" json:"ldap_group"`
}

// MemberList parses the stored distribution-group member list.
func (a *Alias) MemberList() []AliasMember {
  if a.Members == "" {
    return nil
  }
  var out []AliasMember
  if err := json.Unmarshal([]byte(a.Members), &out); err != nil {
    return nil
  }
  return out
}

// SetMembers encodes the distribution-group member list for storage.
func (a *Alias) SetMembers(members []AliasMember) {
  if len(members) == 0 {
    a.Members = ""
    return
  }
  if b, err := json.Marshal(members); err == nil {
    a.Members = string(b)
  }
}

// MarshalJSON exposes the parsed member array (and hides the raw JSON column).
func (a Alias) MarshalJSON() ([]byte, error) {
  type plain Alias
  v := struct {
    plain
    Members []AliasMember `json:"members"`
  }{plain: plain(a), Members: a.MemberList()}
  return json.Marshal(v)
}

// Destinations returns the parsed destination list.
func (a *Alias) Destinations() []string {
  return splitCSV(a.Destination)
}

// Targets returns every delivery target: the destination list plus the
// distribution-group members.
func (a *Alias) Targets() []string {
  targets := a.Destinations()
  for _, m := range a.MemberList() {
    if strings.TrimSpace(m.Email) != "" {
      targets = append(targets, m.Email)
    }
  }
  return uniqueStrings(targets)
}

// uniqueStrings deduplicates addresses case-insensitively, preserving order.
func uniqueStrings(items []string) []string {
  seen := make(map[string]bool, len(items))
  out := make([]string, 0, len(items))
  for _, it := range items {
    k := strings.ToLower(strings.TrimSpace(it))
    if k == "" || seen[k] {
      continue
    }
    seen[k] = true
    out = append(out, it)
  }
  return out
}
