//go:build mailez_ee

package user

import (
	"gorm.io/gorm"

	"mailez/backend/internal/ee/license"
)

// capacityCheck enforces the licensed mailbox cap on account creation.
func capacityCheck(db *gorm.DB) error {
	return license.FromEnv().CheckCapacity(db)
}
