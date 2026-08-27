package models

import "time"

// DlpAction is what happens when a rule matches an outbound message.
type DlpAction string

// DLP actions.
const (
	DlpActionBlock DlpAction = "block" // reject at submission time
	DlpActionHold  DlpAction = "hold"  // require approval before delivery
)

// DlpRule is one outbound content rule (敏感词/正则) with a compliance
// action. Scope "all" applies to every domain; "domain:<name>" to one.
type DlpRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null;uniqueIndex" json:"name"`
	Enabled   bool      `gorm:"not null" json:"enabled"`
	Pattern   string    `gorm:"size:1024;not null" json:"pattern"`
	IsRegex   bool      `gorm:"not null" json:"is_regex"`
	Scope     string    `gorm:"size:255;not null;default:'all'" json:"scope"` // all | domain:<name>
	Action    DlpAction `gorm:"size:16;not null" json:"action"`
	Severity  string    `gorm:"size:16;not null;default:'medium'" json:"severity"`
	Approvers string    `gorm:"size:2048" json:"approvers"` // comma-separated emails (hold only)
	HoldHours int       `gorm:"not null;default:48" json:"hold_hours"`
	Note      string    `gorm:"size:1024" json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PendingApproval is one held outbound message awaiting an approver's
// decision. The raw RFC 5322 message is kept for review and later delivery.
type PendingApproval struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	RuleName    string     `gorm:"size:255;not null" json:"rule_name"`
	Severity    string     `gorm:"size:16" json:"severity"`
	SenderEmail string     `gorm:"size:255;not null;index" json:"sender_email"`
	From        string     `gorm:"size:512;not null" json:"from"`
	Recipients  string     `gorm:"size:2048;not null" json:"recipients"`
	Subject     string     `gorm:"size:1024" json:"subject"`
	Raw         []byte     `gorm:"type:blob" json:"-"`
	Status      string     `gorm:"size:16;not null;default:'pending';index" json:"status"`
	Approver    string     `gorm:"size:255" json:"approver"`
	DecisionAt  *time.Time `json:"decision_at"`
	Reason      string     `gorm:"size:2048" json:"reason"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `gorm:"index" json:"expires_at"`
}

// DlpCheckResult is the stack API response to an engine scan request.
type DlpCheckResult struct {
	Action string `json:"action"` // pass | block | hold
	Reason string `json:"reason,omitempty"`
	ID     uint   `json:"id,omitempty"` // pending approval id when held
}
