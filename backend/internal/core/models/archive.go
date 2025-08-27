package models

import "time"

// ArchiveDirection is the capture direction of an archived message.
type ArchiveDirection string

// Capture directions.
const (
	ArchiveInbound  ArchiveDirection = "inbound"
	ArchiveOutbound ArchiveDirection = "outbound"
)

// ArchiveSettings is the capture/retention policy for one scope. The global
// row has Domain == ""; a per-domain row overrides it for that domain.
type ArchiveSettings struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Domain          string    `gorm:"size:255;uniqueIndex" json:"domain"` // "" = global
	Enabled         bool      `gorm:"not null" json:"enabled"`
	CaptureInbound  bool      `gorm:"not null" json:"capture_inbound"`
	CaptureOutbound bool      `gorm:"not null" json:"capture_outbound"`
	RetentionDays   int       `gorm:"not null;default:0" json:"retention_days"` // 0 = keep forever
	UpdatedAt       time.Time `json:"updated_at"`
}

// ArchivedMessage is one captured message available for compliance review.
// Raw holds the full RFC 5322 message; metadata columns keep the list
// searchable without parsing every row.
type ArchivedMessage struct {
	ID           uint             `gorm:"primaryKey" json:"id"`
	Direction    ArchiveDirection `gorm:"size:16;not null;index" json:"direction"`
	Domain       string           `gorm:"size:255;index" json:"domain"`
	EnvelopeFrom string           `gorm:"size:512;index" json:"envelope_from"`
	EnvelopeTo   string           `gorm:"size:2048" json:"envelope_to"`
	MessageID    string           `gorm:"size:512;index" json:"message_id"`
	From         string           `gorm:"size:512;index" json:"from"`
	To           string           `gorm:"size:2048" json:"to"`
	Cc           string           `gorm:"size:2048" json:"cc"`
	// size:512 keeps the utf8mb4 index under MySQL's 3072-byte key limit
	// (512×4 = 2048 bytes); 1024 would exceed it on MySQL 8.0.
	Subject      string           `gorm:"size:512;index" json:"subject"`
	Date         time.Time        `gorm:"index" json:"date"`
	Size         int64            `gorm:"not null" json:"size"`
	Raw          []byte           `gorm:"type:blob" json:"-"`
	ArchivedAt   time.Time        `gorm:"index" json:"archived_at"`
	ExpiresAt    *time.Time       `gorm:"index" json:"expires_at"`
	Reviewed     bool             `gorm:"not null;default:false;index" json:"reviewed"`
	ReviewedBy   string           `gorm:"size:255" json:"reviewed_by"`
	ReviewedAt   *time.Time       `json:"reviewed_at"`
	ReviewNote   string           `gorm:"size:4096" json:"review_note"`
}

// EffectiveRetention returns the retention deadline for a message stored
// with the given policy, or nil when it must be kept forever.
func (a *ArchiveSettings) EffectiveRetention(now time.Time) *time.Time {
	if !a.Enabled || a.RetentionDays <= 0 {
		return nil
	}
	exp := now.AddDate(0, 0, a.RetentionDays)
	return &exp
}

// Captures reports whether a message with the given direction should be
// stored under this policy.
func (a *ArchiveSettings) Captures(dir ArchiveDirection) bool {
	if !a.Enabled {
		return false
	}
	switch dir {
	case ArchiveInbound:
		return a.CaptureInbound
	case ArchiveOutbound:
		return a.CaptureOutbound
	}
	return false
}
