package models

import "time"

// UploadedFile is one large attachment stored by the relay (超大附件): files
// over the inline cap are mailed as a token-protected download link.
type UploadedFile struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserEmail   string     `gorm:"size:255;not null;index:idx_uploads_user" json:"user_email"`
	Filename    string     `gorm:"size:512;not null" json:"filename"`
	ContentType string     `gorm:"size:255" json:"content_type"`
	Size        int64      `gorm:"not null" json:"size"`
	SHA256      string     `gorm:"size:64" json:"sha256"`
	StoredPath  string     `gorm:"size:1024;not null" json:"-"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}
