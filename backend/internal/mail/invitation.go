package mail

import (
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// Invitation is the structured view of a text/calendar part in a message
// (RFC 5546 iTIP): a meeting REQUEST, a REPLY, or a CANCEL.
type Invitation struct {
	Method      string   `json:"method"` // REQUEST | REPLY | CANCEL
	UID         string   `json:"uid"`
	Summary     string   `json:"summary"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Start       string   `json:"start,omitempty"` // RFC3339 (empty for all-day)
	End         string   `json:"end,omitempty"`
	AllDay      bool     `json:"all_day"`
	Organizer   string   `json:"organizer"`
	Attendees   []string `json:"attendees"`
	ICS         string   `json:"ics"`
}

// ParseInvitation extracts the invitation from raw text/calendar bytes.
// Returns nil when the content is not a usable iCalendar message.
func ParseInvitation(raw []byte) *Invitation {
	cal, err := ics.ParseCalendar(strings.NewReader(string(raw)))
	if err != nil {
		return nil
	}
	out := &Invitation{
		ICS: string(raw),
	}
	for _, p := range cal.CalendarProperties {
		if p.IANAToken == string(ics.ComponentPropertyMethod) {
			out.Method = strings.ToUpper(strings.TrimSpace(p.Value))
			break
		}
	}
	ev := cal.Events()
	if len(ev) == 0 {
		return nil
	}
	e := ev[0]
	prop := func(p ics.ComponentProperty) string {
		if v := e.GetProperty(p); v != nil {
			return strings.TrimSpace(v.Value)
		}
		return ""
	}
	out.UID = prop(ics.ComponentPropertyUniqueId)
	out.Summary = prop(ics.ComponentPropertySummary)
	out.Location = prop(ics.ComponentPropertyLocation)
	out.Description = prop(ics.ComponentPropertyDescription)
	if org := e.GetProperty(ics.ComponentPropertyOrganizer); org != nil {
		out.Organizer = addressValue(org.Value)
	}
	for _, at := range e.Attendees() {
		if a := addressValue(at.Value); a != "" {
			out.Attendees = append(out.Attendees, a)
		}
	}
	if st := e.GetProperty(ics.ComponentPropertyDtStart); st != nil {
		t, allDay := parseICSTime(st.Value, st.ICalParameters)
		out.Start = t
		out.AllDay = allDay
	}
	if en := e.GetProperty(ics.ComponentPropertyDtEnd); en != nil {
		t, allDay := parseICSTime(en.Value, en.ICalParameters)
		out.End = t
		out.AllDay = out.AllDay || allDay
	}
	if out.UID == "" {
		return nil
	}
	return out
}

// parseICSTime renders an iCalendar date-time into RFC3339 ("" when the
// value cannot be parsed) plus the all-day flag.
func parseICSTime(value string, params map[string][]string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) == 8 { // YYYYMMDD all-day
		if t, err := time.Parse("20060102", value); err == nil {
			return t.UTC().Format("2006-01-02"), true
		}
	}
	for _, v := range params["VALUE"] {
		if strings.EqualFold(v, "DATE") {
			if t, err := time.Parse("20060102", value); err == nil {
				return t.UTC().Format("2006-01-02"), true
			}
		}
	}
	if strings.HasSuffix(value, "Z") {
		if t, err := time.Parse("20060102T150405Z", value); err == nil {
			return t.UTC().Format(time.RFC3339), false
		}
	}
	if tzids := params["TZID"]; len(tzids) > 0 {
		if loc, err := time.LoadLocation(tzids[0]); err == nil {
			if t, err := time.ParseInLocation("20060102T150405", value, loc); err == nil {
				return t.UTC().Format(time.RFC3339), false
			}
		}
	}
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t.UTC().Format(time.RFC3339), false
	}
	return "", false
}

// addressValue strips display-name quoting and angle brackets from a
// cal-address value ("mailto:a@b" / "A <mailto:a@b>").
func addressValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "mailto:")
	v = strings.Trim(v, "\"")
	if i := strings.LastIndex(v, "<"); i >= 0 {
		v = strings.Trim(v[i+1:], ">")
		v = strings.TrimPrefix(strings.TrimSpace(v), "mailto:")
	}
	return strings.TrimSpace(v)
}
