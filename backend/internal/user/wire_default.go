package user

import "gorm.io/gorm"

// capacityCheck applies no mailbox-capacity guard in the base build.
func capacityCheck(db *gorm.DB) error { return nil }
