package caldav

import (
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// EventData is the structured view of one iCalendar VEVENT, extracted for
// storage columns and calendar-query filtering. The raw ICS is always kept
// byte-for-byte so client formatting round-trips untouched.
type EventData struct {
	UID         string
	Summary     string
	Location    string
	Description string
	RRule       string
	AllDay      bool
	Start       *time.Time
	End         *time.Time
}

// ParseEvent extracts the structured fields from a raw iCalendar document.
// A missing DTEND falls back to DTSTART (RFC 5545 zero-duration event).
func ParseEvent(raw string) (*EventData, error) {
	cal, err := ics.ParseCalendar(strings.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid iCalendar: %w", err)
	}
	events := cal.Events()
	if len(events) == 0 {
		return nil, fmt.Errorf("no VEVENT component")
	}
	ev := events[0]
	out := &EventData{}
	prop := func(p ics.ComponentProperty) string {
		if v := ev.GetProperty(p); v != nil {
			return v.Value
		}
		return ""
	}
	out.UID = prop(ics.ComponentPropertyUniqueId)
	out.Summary = prop(ics.ComponentPropertySummary)
	out.Location = prop(ics.ComponentPropertyLocation)
	out.Description = prop(ics.ComponentPropertyDescription)
	out.RRule = prop(ics.ComponentPropertyRrule)
	if out.UID == "" {
		return nil, fmt.Errorf("VEVENT is missing UID")
	}
	if st := ev.GetProperty(ics.ComponentPropertyDtStart); st != nil {
		t, allDay := parseTimeValue(st.Value, st.ICalParameters)
		out.Start = &t
		out.AllDay = allDay
	}
	if en := ev.GetProperty(ics.ComponentPropertyDtEnd); en != nil {
		t, allDay := parseTimeValue(en.Value, en.ICalParameters)
		out.End = &t
		out.AllDay = out.AllDay || allDay
	} else if out.Start != nil {
		t := *out.Start
		out.End = &t
	}
	return out, nil
}

// parseTimeValue renders an iCalendar date-time value into a time.Time and
// whether the value is a DATE (all-day). TZID parameters are honoured when
// the zone is known; floating times are treated as UTC for storage.
func parseTimeValue(value string, params map[string][]string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if len(value) == 8 { // YYYYMMDD all-day
		if t, err := time.Parse("20060102", value); err == nil {
			return t, true
		}
	}
	for _, v := range params["VALUE"] {
		if strings.EqualFold(v, "DATE") {
			if t, err := time.Parse("20060102", value); err == nil {
				return t, true
			}
		}
	}
	formats := []string{"20060102T150405Z", "20060102T150405"}
	if strings.HasSuffix(value, "Z") {
		if t, err := time.Parse("20060102T150405Z", value); err == nil {
			return t, false
		}
	}
	if tzids := params["TZID"]; len(tzids) > 0 {
		if loc, err := time.LoadLocation(tzids[0]); err == nil {
			if t, err := time.ParseInLocation("20060102T150405", value, loc); err == nil {
				return t, false
			}
		}
	}
	for _, f := range formats {
		if t, err := time.Parse(f, value); err == nil {
			return t, false
		}
	}
	return time.Time{}, false
}

// BuildICS renders a VEVENT document from structured fields (webmail create).
func BuildICS(d *EventData) (string, error) {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	ev := cal.AddEvent(d.UID)
	ev.SetProperty(ics.ComponentPropertySummary, d.Summary)
	ev.SetProperty(ics.ComponentPropertyDtstamp, time.Now().UTC().Format("20060102T150405Z"))
	if d.Location != "" {
		ev.SetProperty(ics.ComponentPropertyLocation, d.Location)
	}
	if d.Description != "" {
		ev.SetProperty(ics.ComponentPropertyDescription, d.Description)
	}
	if d.Start != nil {
		if d.AllDay {
			ev.SetAllDayStartAt(*d.Start)
			end := d.Start.Add(24 * time.Hour)
			if d.End != nil && !d.End.Equal(*d.Start) {
				end = *d.End
			}
			ev.SetAllDayEndAt(end)
		} else {
			ev.SetStartAt(*d.Start)
			end := d.Start.Add(time.Hour)
			if d.End != nil {
				end = *d.End
			}
			ev.SetEndAt(end)
		}
	}
	if d.RRule != "" {
		ev.SetProperty(ics.ComponentPropertyRrule, d.RRule)
	}
	return cal.Serialize(), nil
}
