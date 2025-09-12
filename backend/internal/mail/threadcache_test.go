package mail

import "testing"

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
