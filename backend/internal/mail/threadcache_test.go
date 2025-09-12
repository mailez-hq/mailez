package mail

import (
	"testing"

	"github.com/emersion/go-imap"
)

// countingFetcher counts scans so the cached wrapper's memoisation is
// observable without an IMAP server.
type countingFetcher struct {
	fakeThreadFetcher
	calls int
}

func (f *countingFetcher) Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error {
	f.calls++
	return f.fakeThreadFetcher.Fetch(seqset, items, ch)
}

func TestThreadMetaCachedSkipsRepeatScan(t *testing.T) {
	threadCacheReset()
	c := &Client{}
	ff := &countingFetcher{fakeThreadFetcher: fakeThreadFetcher{msgs: []*imap.Message{
		env(1, "计划", "root@example.com", "", d(10)),
	}}}

	first, err := c.threadMetaCached(ff, 1, "user@test.dist", "Inbox")
	if err != nil {
		t.Fatalf("threadMetaCached: %v", err)
	}
	scans := ff.calls
	second, err := c.threadMetaCached(ff, 1, "user@test.dist", "Inbox")
	if err != nil {
		t.Fatalf("threadMetaCached (cached): %v", err)
	}
	if ff.calls != scans {
		t.Fatalf("repeat lookup rescanned the mailbox: %d scans, want %d", ff.calls, scans)
	}
	if second != first {
		t.Fatalf("cached lookup returned a different meta: %p vs %p", second, first)
	}
	// A delivery or expunge changes the count: the entry must not be reused.
	if _, err := c.threadMetaCached(ff, 2, "user@test.dist", "Inbox"); err != nil {
		t.Fatalf("threadMetaCached (count changed): %v", err)
	}
	if ff.calls == scans {
		t.Fatal("a changed message count must invalidate the cached entry")
	}
}

func TestThreadCacheHitMissAndTTL(t *testing.T) {
	threadCacheReset()
	if _, ok := threadCacheGet("acct|Inbox|5"); ok {
		t.Fatal("empty cache reported a hit")
	}
	meta := &threadMeta{ids: map[uint32]string{1: "t1"}}
	threadCachePut("acct|Inbox|5", meta)
	got, ok := threadCacheGet("acct|Inbox|5")
	if !ok || got != meta {
		t.Fatalf("expected the stored entry back, got ok=%v meta=%p", ok, got)
	}
	// A different message count is a different mailbox state: must miss.
	if _, ok := threadCacheGet("acct|Inbox|6"); ok {
		t.Fatal("count is part of the key; a changed count must miss")
	}
	// A different folder must miss too.
	if _, ok := threadCacheGet("acct|Trash|5"); ok {
		t.Fatal("folder is part of the key; another folder must miss")
	}
	if n := threadCacheLen(); n != 1 {
		t.Fatalf("cache length = %d, want 1", n)
	}
}
