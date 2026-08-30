package mail

import (
	"testing"
	"time"

	"github.com/emersion/go-imap"
)

// fakeThreadFetcher serves canned envelopes by sequence number so the
// reference-based grouping stays unit-testable without an IMAP server.
type fakeThreadFetcher struct{ msgs []*imap.Message }

func (f *fakeThreadFetcher) Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error {
	defer close(ch)
	for _, m := range f.msgs {
		ch <- m
	}
	return nil
}

func env(uid uint32, subject, msgid, inReplyTo string, date time.Time) *imap.Message {
	m := &imap.Message{Uid: uid, Envelope: &imap.Envelope{Subject: subject, Date: date}}
	if msgid != "" {
		m.Envelope.MessageId = "<" + msgid + ">"
	}
	if inReplyTo != "" {
		m.Envelope.InReplyTo = "<" + inReplyTo + ">"
	}
	return m
}

func nodesOf(t *testing.T, ff *fakeThreadFetcher, total uint32) map[uint32]string {
	t.Helper()
	c := &Client{}
	meta, err := c.threadMeta(ff, total)
	if err != nil {
		t.Fatalf("threadMeta: %v", err)
	}
	return meta.ids
}

func d(minute int) time.Time { return time.Date(2026, 8, 29, 17, minute, 0, 0, time.UTC) }

// A reply chain links into one component keyed by the root's message-id,
// while an unrelated message that merely shares the subject stays separate.
func TestGroupThreadsSplitsSameSubjectDistinctChains(t *testing.T) {
	ff := &fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "计划", "root-a@example.com", "", d(10)),
		env(2, "Re: 计划", "r1@example.com", "root-a@example.com", d(11)),
		env(3, "Re: Re: 计划", "r2@example.com", "r1@example.com", d(12)),
		env(4, "计划", "root-b@example.com", "", d(13)), // different conversation
	}}
	ids := nodesOf(t, ff, 4)
	if ids[1] != ids[2] || ids[2] != ids[3] {
		t.Fatalf("chain split: %v", ids)
	}
	if ids[1] == ids[4] {
		t.Fatalf("same-subject distinct conversations merged: %v", ids)
	}
	if ids[1] != "m:<root-a@example.com>" {
		t.Fatalf("chain key = %q, want root msgid", ids[1])
	}
	if ids[4] != "s:计划" {
		t.Fatalf("unlinked key = %q, want subject fallback", ids[4])
	}
}

// Mails without any usable references keep merging by subject — the
// behaviour every existing thread view relies on.
func TestGroupThreadsSubjectFallbackWithoutRefs(t *testing.T) {
	ff := &fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "Re: Hello", "a@example.com", "", d(10)),
		env(2, "FW: hello", "b@example.com", "", d(11)),
		env(3, "Re: Re: HELLO", "c@example.com", "", d(12)),
	}}
	ids := nodesOf(t, ff, 3)
	if ids[1] != ids[2] || ids[2] != ids[3] {
		t.Fatalf("subject fallback no longer merges: %v", ids)
	}
	if ids[1] != "s:hello" {
		t.Fatalf("fallback key = %q, want s:hello", ids[1])
	}
}

// The webmail quick-reply path historically sent the base64url routable id
// as In-Reply-To; such values must not link (they match no Message-ID) and
// must degrade to the subject merge instead of orphaning the reply.
func TestGroupThreadsGarbageInReplyToDegradesToSubject(t *testing.T) {
	ff := &fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "Hi", "orig@example.com", "", d(10)),
		env(2, "Re: Hi", "reply@example.com", "YnJva2VuLWJhc2U2", d(11)),
	}}
	ids := nodesOf(t, ff, 2)
	if ids[1] != ids[2] {
		t.Fatalf("garbage ref orphaned the reply: %v", ids)
	}
}

// A reply whose parent scrolled out of the scanned window falls back to the
// subject key, so it still groups with the in-window members of its own
// conversation.
func TestGroupThreadsParentOutsideWindowUsesSubject(t *testing.T) {
	ff := &fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "Re: Old", "a@example.com", "ancient@example.com", d(10)),
		env(2, "Re: Old", "b@example.com", "a@example.com", d(11)),
	}}
	ids := nodesOf(t, ff, 2)
	if ids[1] != ids[2] {
		t.Fatalf("window-edge chain split: %v", ids)
	}
	if ids[1] != "m:<a@example.com>" {
		t.Fatalf("in-window chain key = %q, want oldest member msgid", ids[1])
	}
}

// Mutual (malformed) references must not wedge the union-find walk.
func TestGroupThreadsMutualRefsDoNotCycle(t *testing.T) {
	ff := &fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "X", "a@example.com", "b@example.com", d(10)),
		env(2, "Re: X", "b@example.com", "a@example.com", d(11)),
		env(3, "Re: X", "c@example.com", "b@example.com", d(12)),
	}}
	ids := nodesOf(t, ff, 3)
	if ids[1] != ids[2] || ids[2] != ids[3] {
		t.Fatalf("mutual refs split the group: %v", ids)
	}
}

// counts/latest drive the conversation badges and the "which row is newest"
// logic in the list.
func TestThreadMetaCountsAndLatest(t *testing.T) {
	ff := &fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "A", "a1@example.com", "", d(10)),
		env(2, "Re: A", "a2@example.com", "a1@example.com", d(11)),
		env(3, "Re: A", "a3@example.com", "a2@example.com", d(12)),
		env(4, "B", "b1@example.com", "", d(13)),
		env(5, "Re: B", "b2@example.com", "b1@example.com", d(14)),
	}}
	c := &Client{}
	meta, err := c.threadMeta(ff, 5)
	if err != nil {
		t.Fatalf("threadMeta: %v", err)
	}
	if got := meta.counts[ids1(t, meta, 1)]; got != 3 {
		t.Fatalf("thread A count = %d, want 3", got)
	}
	if got := meta.counts[ids1(t, meta, 4)]; got != 2 {
		t.Fatalf("thread B count = %d, want 2", got)
	}
	if meta.latest[ids1(t, meta, 1)] != 3 || meta.latest[ids1(t, meta, 4)] != 5 {
		t.Fatalf("latest = %v/%v, want 3/5", meta.latest[ids1(t, meta, 1)], meta.latest[ids1(t, meta, 4)])
	}
}

func ids1(t *testing.T, meta *threadMeta, uid uint32) string {
	t.Helper()
	v, ok := meta.ids[uid]
	if !ok {
		t.Fatalf("uid %d has no thread id", uid)
	}
	return v
}
