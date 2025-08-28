package authcache

import (
	"testing"
	"time"
)

func TestCacheHitsAndMisses(t *testing.T) {
	c := New(time.Minute)
	var calls int
	verify := func() bool {
		calls++
		return true
	}
	if !c.Check("a@example.com", "pw1", verify) {
		t.Fatal("first check must verify")
	}
	if !c.Check("a@example.com", "pw1", verify) {
		t.Fatal("second check must hit the cache")
	}
	if calls != 1 {
		t.Fatalf("verify called %d times, want 1 (cached)", calls)
	}
	// A different credential pair must not ride the cached success.
	if c.Check("a@example.com", "pw2", func() bool {
		calls++
		return false
	}) {
		t.Fatal("different password must fail")
	}
	if c.Check("b@example.com", "pw1", func() bool {
		calls++
		return true
	}) != true {
		t.Fatal("different user must verify")
	}
	if calls != 3 {
		t.Fatalf("verify called %d times, want 3", calls)
	}
}

func TestCacheNeverCachesFailures(t *testing.T) {
	c := New(time.Minute)
	var calls int
	fail := func() bool {
		calls++
		return false
	}
	if c.Check("a@example.com", "wrong", fail) {
		t.Fatal("wrong password must fail")
	}
	if c.Check("a@example.com", "wrong", fail) {
		t.Fatal("wrong password must fail again (failures not cached)")
	}
	if calls != 2 {
		t.Fatalf("verify called %d times, want 2 (no negative cache)", calls)
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	c := New(20 * time.Millisecond)
	var calls int
	ok := func() bool {
		calls++
		return true
	}
	if !c.Check("a@example.com", "pw", ok) {
		t.Fatal("first check must verify")
	}
	time.Sleep(30 * time.Millisecond)
	if !c.Check("a@example.com", "pw", ok) {
		t.Fatal("expired entry must re-verify")
	}
	if calls != 2 {
		t.Fatalf("verify called %d times, want 2 (TTL expired)", calls)
	}
}

func TestCacheCapacityStopsCachingNotVerifying(t *testing.T) {
	// maxCachedKeys is a package constant; simulate pressure by filling the
	// map directly and confirming the cache degrades to verify-every-time.
	c := New(time.Minute)
	for i := 0; i < maxCachedKeys; i++ {
		k := credentialKey("u@example.com", string(rune(i)))
		c.m[k] = cacheEntry{exp: time.Now().Add(time.Minute)}
	}
	var calls int
	if !c.Check("a@example.com", "pw", func() bool {
		calls++
		return true
	}) {
		t.Fatal("must verify when at capacity")
	}
	if calls != 1 {
		t.Fatalf("verify called %d times, want 1", calls)
	}
}

func TestCacheSweepsExpiredOnPressure(t *testing.T) {
	c := New(time.Minute)
	for i := 0; i < maxCachedKeys; i++ {
		k := credentialKey("u@example.com", string(rune(i)))
		c.m[k] = cacheEntry{exp: time.Now().Add(-time.Second)} // all expired
	}
	var calls int
	if !c.Check("a@example.com", "pw", func() bool {
		calls++
		return true
	}) {
		t.Fatal("must verify")
	}
	if calls != 1 {
		t.Fatalf("verify called %d times, want 1", calls)
	}
	// The sweeper should have freed room and the success should be cached.
	if c.Check("a@example.com", "pw", func() bool {
		calls++
		return true
	}) != true {
		t.Fatal("must hit cache after sweep")
	}
	if calls != 1 {
		t.Fatalf("verify called %d times after sweep, want 1 (cached)", calls)
	}
}
