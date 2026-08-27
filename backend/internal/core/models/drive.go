package models

import "time"

// DriveFile is one entry of the built-in cloud drive (云盘): folders form a
// tree via ParentID (0 = root); file blobs live behind the drive storage
// backend (local disk or MinIO/S3), referenced by StoredPath.
type DriveFile struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserEmail   string    `gorm:"size:255;not null;index:idx_drive_user_parent,priority:1" json:"-"`
	ParentID    uint      `gorm:"not null;default:0;index:idx_drive_user_parent,priority:2" json:"parent_id"`
	Name        string    `gorm:"size:512;not null" json:"name"`
	IsDir       bool      `gorm:"not null;default:false" json:"is_dir"`
	Size        int64     `gorm:"not null;default:0" json:"size"`
	ContentType string    `gorm:"size:255" json:"content_type"`
	SHA256      string    `gorm:"size:64" json:"sha256"`
	StoredPath  string    `gorm:"size:1024" json:"-"`
	ShareToken  string    `gorm:"size:64;index:idx_drive_share_token" json:"-"`
	Trashed     bool      `gorm:"not null;default:false;index" json:"trashed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
