package mail

import (
	"sync"
	"time"
)

// threadCache memoises threadMeta per (account|folder|message count).
//
// Building thread metadata scans up to threadWindow envelopes, and the list
// path does it on every page request: on the demo mailbox that is ~0.4s of the
// ~1.4s a Trash page costs. The count is part of the key, so any delivery or
// expunge invalidates the entry by construction; a same-count swap (one
// message out, one in) can stay briefly stale, which only shifts the "N in
// thread" badge — no message data is affected. Anything stricter would key on
// the mailbox modseq, which this layer does not expose yet.
//
// It lives at package level on purpose: Client.With() hands out a fresh
// *Client copy for every request and copies no cache fields, so a per-Client
// cache would always miss.
var threadCache = struct {
	mu sync.Mutex
	en map[string]threadCacheEntry
}{en: map[string]threadCacheEntry{}}

type threadCacheEntry struct {
	meta   *threadMeta
	minted time.Time
}

// threadCacheTTL matches the message-id cache: long enough to cover a burst of
// page loads, short enough that a same-count swap self-heals.
const threadCacheTTL = 5 * time.Minute

// threadCacheGet returns a fresh entry for key, if any.
func threadCacheGet(key string) (*threadMeta, bool) {
	threadCache.mu.Lock()
	defer threadCache.mu.Unlock()
	e, ok := threadCache.en[key]
	if !ok || time.Since(e.minted) >= threadCacheTTL {
		return nil, false
	}
	return e.meta, true
}

// threadCachePut stores meta for key, dropping expired entries opportunistically
// so the map cannot grow without bound across mailboxes.
func threadCachePut(key string, meta *threadMeta) {
	threadCache.mu.Lock()
	defer threadCache.mu.Unlock()
	now := time.Now()
	for k, e := range threadCache.en {
		if now.Sub(e.minted) >= threadCacheTTL {
			delete(threadCache.en, k)
		}
	}
	threadCache.en[key] = threadCacheEntry{meta: meta, minted: now}
}

// ThreadCacheLen reports the number of live entries (diagnostics/tests).
func threadCacheLen() int {
	threadCache.mu.Lock()
	defer threadCache.mu.Unlock()
	return len(threadCache.en)
}

// threadCacheReset clears the cache (tests).
func threadCacheReset() {
	threadCache.mu.Lock()
	defer threadCache.mu.Unlock()
	threadCache.en = map[string]threadCacheEntry{}
}
