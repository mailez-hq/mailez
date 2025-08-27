// Package calendar exposes the webmail JSON API over the same CalendarEvent
// store the built-in CalDAV server uses, so the web UI and phone/desktop DAV
// clients see one consistent calendar.
package calendar

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/caldav"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// eventInput is the create/update payload from the webmail UI.
type eventInput struct {
	Summary     string `json:"summary"`
	Location    string `json:"location"`
	Description string `json:"description"`
	AllDay      bool   `json:"all_day"`
	Start       string `json:"start"` // RFC3339
	End         string `json:"end"`   // RFC3339 (optional)
	RRule       string `json:"rrule"`
	// ReminderMinutes asks for a reminder this many minutes before start
	// (0 disables it). The reminder is delivered as mail to the owner.
	ReminderMinutes int `json:"reminder_minutes"`
}

// apply validates the input, builds the raw ICS and fills the model row.
func (in *eventInput) apply(ev *models.CalendarEvent) error {
	in.Summary = strings.TrimSpace(in.Summary)
	in.Location = strings.TrimSpace(in.Location)
	in.RRule = strings.TrimSpace(in.RRule)
	if in.Summary == "" || in.Start == "" {
		return errBad("summary and start are required")
	}
	start, err := time.Parse(time.RFC3339, in.Start)
	if err != nil {
		return errBad("invalid start time")
	}
	var end *time.Time
	if in.End != "" {
		t, err := time.Parse(time.RFC3339, in.End)
		if err != nil {
			return errBad("invalid end time")
		}
		end = &t
	}
	if ev.UID == "" {
		ev.UID = newUID()
	}
	ev.ReminderMinutes = in.ReminderMinutes
	if ev.ReminderMinutes < 0 {
		ev.ReminderMinutes = 0
	}
	d := &caldav.EventData{
		UID:         ev.UID,
		Summary:     in.Summary,
		Location:    in.Location,
		Description: in.Description,
		RRule:       in.RRule,
		AllDay:      in.AllDay,
		Start:       &start,
		End:         end,
	}
	icsText, err := caldav.BuildICS(d)
	if err != nil {
		return err
	}
	parsed, err := caldav.ParseEvent(icsText)
	if err != nil {
		return err
	}
	ev.Summary = parsed.Summary
	ev.Location = parsed.Location
	ev.Description = parsed.Description
	ev.AllDay = parsed.AllDay
	ev.Start = parsed.Start
	ev.End = parsed.End
	ev.RRule = parsed.RRule
	ev.ICS = icsText
	return nil
}

type apiError struct{ msg string }

func (e *apiError) Error() string { return e.msg }
func errBad(msg string) error     { return &apiError{msg: msg} }

// newUID generates a random iCalendar UID.
func newUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "mailez-" + time.Now().Format("20060102150405")
	}
	return "mailez-" + hex.EncodeToString(b)
}

// view is the API shape of an event.
func view(ev *models.CalendarEvent) fiber.Map {
	out := fiber.Map{
		"id":               ev.ID,
		"uid":              ev.UID,
		"summary":          ev.Summary,
		"location":         ev.Location,
		"description":      ev.Description,
		"all_day":          ev.AllDay,
		"rrule":            ev.RRule,
		"reminder_minutes": ev.ReminderMinutes,
		"owner_email":      ev.UserEmail,
		"read_only":        false,
		"updated_at":       ev.UpdatedAt,
	}
	if ev.Start != nil {
		out["start"] = ev.Start.Format(time.RFC3339)
	}
	if ev.End != nil {
		out["end"] = ev.End.Format(time.RFC3339)
	}
	return out
}

// Handler serves the calendar routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

// Register mounts the calendar API under the authenticated group.
func (h *Handler) Register(r fiber.Router) {
	r.Get("/calendar/events", h.listEvents)
	r.Post("/calendar/events", h.createEvent)
	r.Put("/calendar/events/:id", h.updateEvent)
	r.Delete("/calendar/events/:id", h.deleteEvent)
	r.Get("/calendar/shares", h.listShares)
	r.Post("/calendar/shares", h.createShare)
	r.Delete("/calendar/shares/:id", h.deleteShare)
	r.Get("/calendar/feed", h.calendarFeed)
	r.Get("/calendar/export.ics", h.exportICS)
}
