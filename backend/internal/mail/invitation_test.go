package mail

import (
	"strings"
	"testing"
)

const requestICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Test//Test//EN\r\n" +
	"METHOD:REQUEST\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:meet-123@example.com\r\n" +
	"DTSTAMP:20260827T000000Z\r\n" +
	"DTSTART:20260901T020000Z\r\n" +
	"DTEND:20260901T030000Z\r\n" +
	"SUMMARY:季度评审\r\n" +
	"LOCATION:会议室A\r\n" +
	"ORGANIZER;CN=Boss:mailto:boss@example.com\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT:mailto:alice@example.com\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseInvitationRequest(t *testing.T) {
	inv := ParseInvitation([]byte(requestICS))
	if inv == nil {
		t.Fatal("parse returned nil")
	}
	if inv.Method != "REQUEST" {
		t.Errorf("method = %q", inv.Method)
	}
	if inv.UID != "meet-123@example.com" {
		t.Errorf("uid = %q", inv.UID)
	}
	if inv.Summary != "季度评审" {
		t.Errorf("summary = %q", inv.Summary)
	}
	if inv.Organizer != "boss@example.com" {
		t.Errorf("organizer = %q", inv.Organizer)
	}
	if len(inv.Attendees) != 1 || inv.Attendees[0] != "alice@example.com" {
		t.Errorf("attendees = %v", inv.Attendees)
	}
	if inv.Start == "" || inv.End == "" {
		t.Errorf("start/end not parsed: %q %q", inv.Start, inv.End)
	}
	if !strings.HasPrefix(inv.Start, "2026-09-01T02:00") {
		t.Errorf("start = %q", inv.Start)
	}
}

func TestParseInvitationGarbage(t *testing.T) {
	if ParseInvitation([]byte("not an ics")) != nil {
		t.Fatal("expected nil for garbage")
	}
}

const replyICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Test//Test//EN\r\n" +
	"METHOD:REPLY\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:meet-123@example.com\r\n" +
	"DTSTAMP:20260827T000000Z\r\n" +
	"ORGANIZER:mailto:boss@example.com\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:alice@example.com\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

const cancelICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Test//Test//EN\r\n" +
	"METHOD:CANCEL\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:meet-123@example.com\r\n" +
	"DTSTAMP:20260827T000000Z\r\n" +
	"ORGANIZER:mailto:boss@example.com\r\n" +
	"ATTENDEE:mailto:alice@example.com\r\n" +
	"STATUS:CANCELLED\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseInvitationReplyPartstat(t *testing.T) {
	inv := ParseInvitation([]byte(replyICS))
	if inv == nil {
		t.Fatal("parse returned nil")
	}
	if inv.Method != "REPLY" {
		t.Fatalf("method = %q", inv.Method)
	}
	if inv.ReplyAttendee != "alice@example.com" {
		t.Errorf("reply attendee = %q", inv.ReplyAttendee)
	}
	if inv.ReplyStatus != "accepted" {
		t.Errorf("reply status = %q", inv.ReplyStatus)
	}
}

func TestParseInvitationCancel(t *testing.T) {
	inv := ParseInvitation([]byte(cancelICS))
	if inv == nil {
		t.Fatal("parse returned nil")
	}
	if inv.Method != "CANCEL" || inv.UID != "meet-123@example.com" {
		t.Fatalf("invitation = %+v", inv)
	}
	if inv.ReplyStatus != "" {
		t.Errorf("cancel must not carry a partstat: %q", inv.ReplyStatus)
	}
}

func TestExtractBodyDetectsCalendarPart(t *testing.T) {
	raw := "From: boss@example.com\r\n" +
		"To: alice@example.com\r\n" +
		"Subject: 邀请\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=xyz\r\n" +
		"\r\n" +
		"--xyz\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"请参加会议\r\n" +
		"--xyz\r\n" +
		"Content-Type: text/calendar; method=REQUEST; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n" +
		"\r\n" +
		requestICS +
		"--xyz--\r\n"
	_, _, attachments, inv, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if inv == nil || inv.Method != "REQUEST" || inv.Summary != "季度评审" {
		t.Fatalf("invitation = %+v", inv)
	}
	if len(attachments) != 0 {
		t.Errorf("calendar part must not be listed as an attachment: %v", attachments)
	}
}
