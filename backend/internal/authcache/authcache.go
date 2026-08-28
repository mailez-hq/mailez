// Package authcache memoizes successful HTTP Basic credential checks so
// protocol clients that re-authenticate on every request (CalDAV, CardDAV,
// Exchange ActiveSync, nginx webdav) do not pay a DB lookup plus a cost-12
// bcrypt verification each time.
package authcache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	defaultTTL    = 10 * time.Minute
	maxCachedKeys = 65536
)

// Cache stores only successful verifications, keyed by the credential pair.
// A cache hit replays the exact credentials that already succeeded, so a wrong
// password can never ride a prior success; failures are never cached (brute
// force stays visible to the verifier and rate limiters). The TTL bounds how
// long a password change or account disable takes to propagate, mirroring
// Dovecot's auth_cache_ttl semantics.
type Cache struct {
	mu  sync.Mutex
	m   map[string]cacheEntry
	ttl time.Duration
}

type cacheEntry struct {
	exp time.Time
}

// New returns a cache with the given TTL (zero/negative falls back to 10m).
func New(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Cache{m: make(map[string]cacheEntry), ttl: ttl}
}

// Check returns the cached verdict for the credential pair, or runs verify to
// compute it. Only successes are memoized.
func (c *Cache) Check(email, pw string, verify func() bool) bool {
	key := credentialKey(email, pw)
	now := time.Now()

	c.mu.Lock()
	if e, ok := c.m[key]; ok {
		if now.Before(e.exp) {
			c.mu.Unlock()
			return true
		}
		delete(c.m, key)
	}
	// Bound memory under credential spraying: sweep expired entries first;
	// if still at capacity, stop caching instead of evicting valid entries.
	if len(c.m) >= maxCachedKeys {
		for k, v := range c.m {
			if now.After(v.exp) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxCachedKeys {
			c.mu.Unlock()
			return verify()
		}
	}
	c.mu.Unlock()

	if !verify() {
		return false
	}
	c.mu.Lock()
	c.m[key] = cacheEntry{exp: now.Add(c.ttl)}
	c.mu.Unlock()
	return true
}

// credentialKey derives a cache key from the credential pair. The plaintext
// password is never stored; only its SHA-256 digest participates.
func credentialKey(email, pw string) string {
	sum := sha256.Sum256([]byte(email + "\x00" + pw))
	return hex.EncodeToString(sum[:])
}
