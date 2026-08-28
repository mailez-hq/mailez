package mail

import (
	"strconv"
	"testing"
	"time"
)

func testRegistry(t *testing.T, ttl time.Duration, perUser, global int) *poolRegistry {
	t.Helper()
	t.Setenv("MAILEZ_IMAP_POOL_IDLE", ttl.String())
	t.Setenv("MAILEZ_IMAP_POOL_SIZE", itoa(perUser))
	t.Setenv("MAILEZ_IMAP_POOL_GLOBAL", itoa(global))
	return newPoolRegistry()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestPoolReusesIdleConnection(t *testing.T) {
	reg := testRegistry(t, time.Minute, 2, 4)
	c := New("127.0.0.1:1", "", "")

	first := &pooledConn{pool: &userPool{}, reg: reg, lastUsed: time.Now()}
	// Seed the pool as if a previous operation released the connection.
	reg.pools["a@example.com"] = first.pool
	if err := reg.release(first); err != nil {
		t.Fatalf("release: %v", err)
	}

	got, err := reg.acquire(c, "a@example.com", "token-x")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if got != first {
		t.Fatalf("acquire returned a different connection: want reuse of the idle one")
	}
	// A second acquire with an empty pool must dial (and fail against the
	// unreachable address) instead of returning a stale connection.
	if _, err := reg.acquire(c, "b@example.com", "token-y"); err == nil {
		t.Fatal("expected dial failure for unknown account")
	}
}

func TestPoolClosesStaleConnection(t *testing.T) {
	reg := testRegistry(t, 50*time.Millisecond, 2, 4)
	c := New("127.0.0.1:1", "", "")

	stale := &pooledConn{pool: &userPool{}, reg: reg, lastUsed: time.Now().Add(-time.Hour)}
	reg.pools["a@example.com"] = stale.pool
	stale.pool.idle = append(stale.pool.idle, stale)
	reg.idleCount.Add(1)

	if _, err := reg.acquire(c, "a@example.com", "token-x"); err == nil {
		t.Fatal("expected dial failure: stale connection must be closed, not reused")
	}
	if !stale.dead {
		t.Fatal("stale connection must be closed")
	}
	if reg.idleCount.Load() != 0 {
		t.Fatalf("idle count = %d, want 0 after stale close", reg.idleCount.Load())
	}
}

func TestPoolRespectsPerUserAndGlobalCaps(t *testing.T) {
	reg := testRegistry(t, time.Minute, 1, 1)

	a := &pooledConn{pool: &userPool{}, reg: reg}
	b := &pooledConn{pool: &userPool{}, reg: reg}
	if err := reg.release(a); err != nil {
		t.Fatalf("release a: %v", err)
	}
	if err := reg.release(b); err != nil {
		t.Fatalf("release b: %v", err)
	}
	if !a.dead && !b.dead {
		t.Fatal("one of the two releases must have closed the connection (caps are 1/1)")
	}
	if reg.idleCount.Load() != 1 {
		t.Fatalf("idle count = %d, want 1", reg.idleCount.Load())
	}
}

func TestPoolDoesNotReuseDeadConnection(t *testing.T) {
	reg := testRegistry(t, time.Minute, 2, 4)
	dead := &pooledConn{pool: &userPool{}, reg: reg, dead: true}
	if err := reg.release(dead); err != nil {
		t.Fatalf("release: %v", err)
	}
	if !dead.dead {
		t.Fatal("dead connection must not return to the pool")
	}
	if reg.idleCount.Load() != 0 {
		t.Fatalf("idle count = %d, want 0", reg.idleCount.Load())
	}
}

func TestUnpooledConnLogoutCloses(t *testing.T) {
	pc := &pooledConn{} // no pool/registry → plain close, nil client is a no-op
	if err := pc.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
}

func TestPoolConfigEnvParsing(t *testing.T) {
	t.Setenv("MAILEZ_IMAP_POOL_IDLE", "bogus")
	t.Setenv("MAILEZ_IMAP_POOL_SIZE", "abc")
	t.Setenv("MAILEZ_IMAP_POOL_GLOBAL", "-3")
	reg := newPoolRegistry()
	if reg.idleTTL != defaultPoolIdleTTL || reg.idleMaxPerUser != defaultPoolIdlePerUser || reg.globalIdleMax != defaultPoolGlobalIdle {
		t.Fatalf("invalid env must fall back to defaults: ttl=%v perUser=%d global=%d",
			reg.idleTTL, reg.idleMaxPerUser, reg.globalIdleMax)
	}
}
