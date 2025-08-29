//go:build mailez_ee

package ldap

import (
	"gorm.io/gorm"
)

// SetCapacityChecker installs the licensed-capacity guard consulted before
// auto-provisioning a directory account (core.DirectorySync seam).
func (s *Service) SetCapacityChecker(fn func(db *gorm.DB) error) {
	s.CheckCapacity = fn
}

// Compile-time seam checks: *Service satisfies the CE interfaces it is
// wired into (core.DirectorySync, auth.LDAPAuthenticator,
// stack.GroupResolver). The assertions live here so the community build
// never needs the ldap import to type-check the seams.
var (
	_ interface {
		SetCapacityChecker(func(*gorm.DB) error)
	} = (*Service)(nil)
)
