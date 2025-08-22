package mail

import (
	"testing"
	"time"
)

func TestParseSearchQuery(t *testing.T) {
	q := parseSearchQuery(`from:amy subject:"weekly report" has:attachment before:2026-01-15 after:2026-01-01 hello world`)
	if len(q.From) != 1 || q.From[0] != "amy" {
		t.Errorf("from: %v", q.From)
	}
	if len(q.Subject) != 1 || q.Subject[0] != "weekly report" {
		t.Errorf("subject: %v", q.Subject)
	}
	if !q.HasAttachment {
		t.Error("has:attachment not parsed")
	}
	wantBefore, _ := time.Parse("2006-01-02", "2026-01-15")
	wantAfter, _ := time.Parse("2006-01-02", "2026-01-01")
	if q.Before == nil || !q.Before.Equal(wantBefore) {
		t.Errorf("before: %v", q.Before)
	}
	if q.After == nil || !q.After.Equal(wantAfter) {
		t.Errorf("after: %v", q.After)
	}
	if len(q.Text) != 2 || q.Text[0] != "hello" || q.Text[1] != "world" {
		t.Errorf("text: %v", q.Text)
	}
}

func TestParseSearchQueryPlainText(t *testing.T) {
	q := parseSearchQuery("invoice march")
	if len(q.Text) != 2 {
		t.Fatalf("text tokens: %v", q.Text)
	}
	if len(q.From) != 0 || q.HasAttachment {
		t.Errorf("unexpected fields: %+v", q)
	}
}

func TestNormalizeSubjectAndThreadID(t *testing.T) {
	if normalizeSubject("Re: Re: Hello") != "hello" {
		t.Errorf("normalize: %q", normalizeSubject("Re: Re: Hello"))
	}
	if normalizeSubject("FW:  Weekly Report ") != "weekly report" {
		t.Errorf("normalize fw: %q", normalizeSubject("FW:  Weekly Report "))
	}
	if threadID("Re: Hello") != threadID("hello") {
		t.Error("thread ids must match across reply prefixes")
	}
	if threadID("") != "" || threadID("   ") != "" {
		t.Error("empty subjects must not group")
	}
}
