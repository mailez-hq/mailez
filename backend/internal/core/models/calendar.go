package models

import "time"

// CalendarEvent is one iCalendar (RFC 5545) event stored by the built-in
// CalDAV server and shown in the webmail calendar. The raw ICS text is kept
// byte-for-byte so PUT/GET round-trips never lose client formatting; the
// structured columns back the list/query endpoints and the webmail UI.
type CalendarEvent struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserEmail   string     `gorm:"size:255;not null;uniqueIndex:idx_calendar_event_uid_user,priority:1" json:"user_email"`
	UID         string     `gorm:"size:255;not null;uniqueIndex:idx_calendar_event_uid_user,priority:2" json:"uid"`
	ETag        string     `gorm:"size:64;default:''" json:"-"`
	Summary     string     `gorm:"size:512;default:''" json:"summary"`
	Location    string     `gorm:"size:512;default:''" json:"location"`
	Description string     `gorm:"type:text" json:"description"`
	AllDay      bool       `gorm:"not null;default:false" json:"all_day"`
	Start       *time.Time `json:"start"`
	End         *time.Time `json:"end"`
	RRule       string     `gorm:"size:512;default:''" json:"rrule"` // raw RRULE; clients expand
	ICS         string     `gorm:"type:text;not null" json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
