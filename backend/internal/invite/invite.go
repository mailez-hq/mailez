// Package invite handles meeting invitations (RFC 5546 iTIP): parsing
// received REQUESTs, replying with accept/decline/tentative, and sending
// new invitations with the .ics attached.
package invite

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"

	"mailez/backend/internal/mail"
)

// RespondKind is the attendee's answer.
type RespondKind string

// Response kinds and their PARTSTAT values.
const (
	Accept    RespondKind = "accept"
	Decline   RespondKind = "decline"
	Tentative RespondKind = "tentative"
)

func partStat(k RespondKind) string {
	switch k {
	case Accept:
		return "ACCEPTED"
	case Decline:
		return "DECLINED"
	default:
		return "TENTATIVE"
	}
}

// BuildReplyICS renders an iTIP REPLY for the given invitation and attendee.
func BuildReplyICS(inv *mail.Invitation, attendee string, kind RespondKind) (string, error) {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodReply)
	cal.SetProductId("-//Mailez//Mailezine Mail//CN")
	ev := cal.AddEvent(inv.UID)
	ev.SetProperty(ics.ComponentPropertyDtstamp, time.Now().UTC().Format("20060102T150405Z"))
	if inv.Summary != "" {
		ev.SetProperty(ics.ComponentPropertySummary, inv.Summary)
	}
	if inv.Organizer != "" {
		ev.SetProperty(ics.ComponentPropertyOrganizer, "mailto:"+inv.Organizer)
	}
	ev.AddAttendee("mailto:"+attendee,
		&ics.KeyValues{Key: "PARTSTAT", Value: []string{partStat(kind)}},
		&ics.KeyValues{Key: "ROLE", Value: []string{"REQ-PARTICIPANT"}})
	// Echo the original start/end so the organizer can correlate the reply.
	if inv.Start != "" {
		if inv.AllDay {
			ev.SetProperty(ics.ComponentPropertyDtStart, strings.ReplaceAll(inv.Start, "-", ""))
		} else if t, err := time.Parse(time.RFC3339, inv.Start); err == nil {
			ev.SetStartAt(t)
		}
	}
	if inv.End != "" {
		if inv.AllDay {
			ev.SetProperty(ics.ComponentPropertyDtEnd, strings.ReplaceAll(inv.End, "-", ""))
		} else if t, err := time.Parse(time.RFC3339, inv.End); err == nil {
			ev.SetEndAt(t)
		}
	}
	return cal.Serialize(ics.WithNewLineWindows), nil
}

// BuildRequestICS renders an iTIP REQUEST for a new meeting.
func BuildRequestICS(uid, summary, location, description string, start, end time.Time, organizer string, attendees []string) (string, error) {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodRequest)
	cal.SetProductId("-//Mailez//Mailezine Mail//CN")
	ev := cal.AddEvent(uid)
	ev.SetProperty(ics.ComponentPropertyUniqueId, uid)
	ev.SetProperty(ics.ComponentPropertyDtstamp, time.Now().UTC().Format("20060102T150405Z"))
	ev.SetProperty(ics.ComponentPropertySummary, summary)
	if location != "" {
		ev.SetProperty(ics.ComponentPropertyLocation, location)
	}
	if description != "" {
		ev.SetProperty(ics.ComponentPropertyDescription, description)
	}
	ev.SetOrganizer("mailto:" + organizer)
	for _, a := range attendees {
		ev.AddAttendee("mailto:"+a,
			&ics.KeyValues{Key: "PARTSTAT", Value: []string{"NEEDS-ACTION"}},
			&ics.KeyValues{Key: "ROLE", Value: []string{"REQ-PARTICIPANT"}})
	}
	ev.SetStartAt(start)
	ev.SetEndAt(end)
	return cal.Serialize(ics.WithNewLineWindows), nil
}

// newUID builds a reasonably unique event UID.
func newUID(salt string) string {
	return fmt.Sprintf("%d-%s@mailez", time.Now().UnixNano(), sanitize(salt))
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "invite"
	}
	return b.String()
}

// asAttachment wraps the ICS as a text/calendar attachment for the composer.
func asAttachment(icsText string) mail.Attachment {
	return mail.Attachment{
		Filename:    "invite.ics",
		ContentType: "text/calendar; method=REQUEST",
		Size:        len(icsText),
		Data:        base64.StdEncoding.EncodeToString([]byte(icsText)),
	}
}
