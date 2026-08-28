package models

import "time"

// WorkerToken stores the encrypted per-user app token that background workers
// (the outbox send-undo queue) use to authenticate as the user over IMAP,
// where no live session credential exists. One token per user, minted on first
// use; the plaintext secret lives only in the worker's memory. The matching
// Token row (IP "outbox-worker") is what the mail proxy validates on login.
type WorkerToken struct {
	UserEmail string    `gorm:"primaryKey;size:255" json:"user_email"`
	TokenEnc  string    `gorm:"type:text" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
