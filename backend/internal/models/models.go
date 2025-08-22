package models

import "gorm.io/gorm"

// AutoMigrate creates or updates all mailez tables.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Config{},
		&Domain{},
		&Alternative{},
		&Relay{},
		&User{},
		&Alias{},
		&Token{},
		&Fetch{},
		&DomainAccess{},
		&AuditLog{},
		&Contact{},
	)
}
