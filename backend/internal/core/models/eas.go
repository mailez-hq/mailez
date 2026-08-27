package models

import "time"

// EasDevice is one registered Exchange ActiveSync client. DeviceID is the
// opaque device identifier the client sends on every command; the unique
// index keeps one row per (user, device).
type EasDevice struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserEmail       string    `gorm:"size:255;not null;uniqueIndex:idx_eas_device_user" json:"user_email"`
	DeviceID        string    `gorm:"size:128;not null;uniqueIndex:idx_eas_device_user" json:"device_id"`
	DeviceType      string    `gorm:"size:128" json:"device_type"`
	ProtocolVersion string    `gorm:"size:16" json:"protocol_version"`
	UserAgent       string    `gorm:"size:512" json:"user_agent"`
	PolicyKey       string    `gorm:"size:128" json:"policy_key"`
	FolderSyncKey   string    `gorm:"size:64" json:"folder_sync_key"`
	FolderSnapshot  string    `gorm:"type:text" json:"folder_snapshot"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// EasSyncState stores the incremental sync snapshot of one collection for one
// device. Snapshot is a JSON document of the folder's items (UID, flags) at
// the last completed sync; SyncKey is the key returned to the client.
type EasSyncState struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserEmail    string    `gorm:"size:255;not null;uniqueIndex:idx_eas_sync_state" json:"user_email"`
	DeviceID     string    `gorm:"size:128;not null;uniqueIndex:idx_eas_sync_state" json:"device_id"`
	CollectionID string    `gorm:"size:255;not null;uniqueIndex:idx_eas_sync_state" json:"collection_id"`
	SyncKey      string    `gorm:"size:64;not null" json:"sync_key"`
	ServerFolder string    `gorm:"size:255" json:"server_folder"`
	UIDValidity  uint32    `json:"uid_validity"`
	Snapshot     string    `gorm:"type:text" json:"snapshot"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EasPingState tracks the folder counters each device last observed, so the
// Ping command can answer "changes?" without a full IMAP walk.
type EasPingState struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserEmail    string    `gorm:"size:255;not null;uniqueIndex:idx_eas_ping_state" json:"user_email"`
	DeviceID     string    `gorm:"size:128;not null;uniqueIndex:idx_eas_ping_state" json:"device_id"`
	CollectionID string    `gorm:"size:255;not null;uniqueIndex:idx_eas_ping_state" json:"collection_id"`
	UidNext      uint32    `json:"uid_next"`
	Messages     uint32    `json:"messages"`
	Unseen       uint32    `json:"unseen"`
	UpdatedAt    time.Time `json:"updated_at"`
}
