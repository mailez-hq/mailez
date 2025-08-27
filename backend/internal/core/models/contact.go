package models

import "time"

// Contact is a personal address-book entry owned by a user.
type Contact struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	UserEmail string `gorm:"size:255;not null;uniqueIndex:idx_contact_dav_uid_user,priority:1" json:"user_email"`
	Name      string `gorm:"size:160;not null" json:"name"`
	Email     string `gorm:"size:255;not null" json:"email"`
	Comment   string `gorm:"size:255;default:''" json:"comment"`
	Groups    string `gorm:"size:255;default:''" json:"groups"`  // comma-separated group names
	Avatar    string `gorm:"size:1024;default:''" json:"avatar"` // URL or data URI
	// DAV sync state: the vCard UID and current ETag exposed through the
	// built-in CardDAV server, plus a monotonically increasing revision used
	// for local-change detection and CTag computation.
	DavUID    string    `gorm:"size:255;uniqueIndex:idx_contact_dav_uid_user,priority:2" json:"-"`
	DavETag   string    `gorm:"size:64;default:''" json:"-"`
	DavRev    int       `gorm:"not null;default:0" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
