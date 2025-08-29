package models

import "time"

// MailDelegation grants one user (Delegate) the ability to act for another
// mailbox owner (Owner). CanSend allows the delegate to send mail as the
// owner; FullAccess additionally lets the delegate open and operate the
// owner's entire mailbox from the webmail (shared/delegated mailbox). A full
// delegation implicitly includes the send right.
type MailDelegation struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	OwnerEmail    string    `gorm:"size:255;not null;uniqueIndex:idx_delegation_owner_delegate" json:"owner_email"`
	DelegateEmail string    `gorm:"size:255;not null;uniqueIndex:idx_delegation_owner_delegate" json:"delegate_email"`
	CanSend       bool      `gorm:"not null;default:false" json:"can_send"`
	FullAccess    bool      `gorm:"not null;default:false" json:"full_access"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
