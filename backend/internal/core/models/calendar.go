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
	// ReminderMinutes is how many minutes before start the reminder fires
	// (0 = no reminder). The reminder is delivered as a mail to the owner,
	// which also triggers the existing web-push path.
	ReminderMinutes int       `gorm:"not null;default:0" json:"reminder_minutes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CalendarShare grants another account read (or read-write) access to a
// user's calendar. The webmail view merges shared events; CalDAV ACLs cover
// DAV clients through the built-in server.
type CalendarShare struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	OwnerEmail  string    `gorm:"size:255;not null;uniqueIndex:idx_calendar_share_pair,priority:1" json:"owner_email"`
	ShareeEmail string    `gorm:"size:255;not null;uniqueIndex:idx_calendar_share_pair,priority:2" json:"sharee_email"`
	ReadOnly    bool      `gorm:"not null" json:"read_only"`
	CreatedAt   time.Time `json:"created_at"`
}

// CalendarReminderLog records that a reminder already fired for an event, so
// the worker never double-sends after a restart.
type CalendarReminderLog struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	EventID   uint       `gorm:"not null;uniqueIndex:idx_calendar_reminder_event" json:"event_id"`
	UserEmail string     `gorm:"size:255;not null" json:"user_email"`
	FireAt    time.Time  `json:"fire_at"`
	SentAt    *time.Time `json:"sent_at"`
	CreatedAt time.Time  `json:"created_at"`
}
