package mail

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/client"
)

// pooledConn wraps an authenticated IMAP connection so that Logout() returns
// the connection to the per-user pool instead of tearing it down. Connections
// without a pool (external aggregated accounts) close for real, preserving the
// old stateless behaviour for third-party servers.
type pooledConn struct {
	*client.Client
	pool *userPool     // nil → plain close on Logout
	reg  *poolRegistry // nil when not pooled

	lastUsed time.Time
	dead     bool
}

// Logout returns the connection to the pool when pooled, otherwise closes it.
// Every existing call site keeps `defer cli.Logout()`, so switching to
// connection reuse is transparent to the mailbox operations.
func (pc *pooledConn) Logout() error {
	if pc.reg == nil || pc.pool == nil {
		if pc.Client == nil {
			return nil
		}
		return pc.Client.Logout()
	}
	return pc.reg.release(pc)
}

// ping verifies the connection is still alive with a NOOP. A connection that
// the server dropped while idle would otherwise fail on the first command of
// the next operation.
func (pc *pooledConn) ping() error {
	if pc.Client == nil {
		return os.ErrClosed
	}
	return pc.Client.Noop()
}

// close permanently closes the underlying connection.
func (pc *pooledConn) close() error {
	pc.dead = true
	if pc.Client == nil {
		return nil
	}
	return pc.Client.Logout()
}

// poolRegistry owns the per-account connection pools of one gateway Client.
// It is shared by With() copies (external dials never touch it) so all
// internal accounts reuse connections through the same bookkeeping.
type poolRegistry struct {
	mu    sync.Mutex
	pools map[string]*userPool

	idleCount      atomic.Int64
	idleMaxPerUser int
	globalIdleMax  int64
	idleTTL        time.Duration
	livenessMinAge time.Duration
}

// userPool holds the idle connections of a single account.
type userPool struct {
	mu   sync.Mutex
	idle []*pooledConn
}

const (
	defaultPoolIdlePerUser = 2
	defaultPoolGlobalIdle  = 128
	defaultPoolIdleTTL     = 5 * time.Minute
)

// newPoolRegistry builds the registry from environment overrides. Keeping idle
// connections per active user is what removes the per-request dial/TLS/login
// round trip; the global cap bounds how many concurrent engine connections the
// pool holds so it stays below the engine's per-listener limit (256 default).
func newPoolRegistry() *poolRegistry {
	return &poolRegistry{
		pools:          make(map[string]*userPool),
		idleMaxPerUser: envInt("MAILEZ_IMAP_POOL_SIZE", defaultPoolIdlePerUser),
		globalIdleMax:  int64(envInt("MAILEZ_IMAP_POOL_GLOBAL", defaultPoolGlobalIdle)),
		idleTTL:        envDuration("MAILEZ_IMAP_POOL_IDLE", defaultPoolIdleTTL),
		livenessMinAge: 2 * time.Second,
	}
}

// acquire returns a live authenticated connection for the account, reusing an
// idle one when possible. Connections idle past idleTTL, or that fail the
// liveness NOOP, are closed and replaced.
func (r *poolRegistry) acquire(c *Client, email, token string) (*pooledConn, error) {
	r.mu.Lock()
	up := r.pools[email]
	if up == nil {
		up = &userPool{}
		r.pools[email] = up
	}
	r.mu.Unlock()

	for {
		up.mu.Lock()
		if len(up.idle) == 0 {
			up.mu.Unlock()
			break
		}
		pc := up.idle[len(up.idle)-1]
		up.idle = up.idle[:len(up.idle)-1]
		up.mu.Unlock()
		r.idleCount.Add(-1)

		if time.Since(pc.lastUsed) > r.idleTTL {
			_ = pc.close()
			continue
		}
		if time.Since(pc.lastUsed) >= r.livenessMinAge {
			if err := pc.ping(); err != nil {
				_ = pc.close()
				continue
			}
		}
		return pc, nil
	}

	cli, err := c.dialIMAP(email, token)
	if err != nil {
		return nil, err
	}
	return &pooledConn{Client: cli, pool: up, reg: r, lastUsed: time.Now()}, nil
}

// release returns a connection to the pool, or closes it when the pool is at
// capacity, the connection is dead, or the global idle budget is exhausted.
func (r *poolRegistry) release(pc *pooledConn) error {
	pc.lastUsed = time.Now()
	keep := !pc.dead && r.idleCount.Load() < r.globalIdleMax
	if keep {
		pc.pool.mu.Lock()
		keep = len(pc.pool.idle) < r.idleMaxPerUser
		if keep {
			pc.pool.idle = append(pc.pool.idle, pc)
		}
		pc.pool.mu.Unlock()
	}
	if !keep {
		return pc.close()
	}
	r.idleCount.Add(1)
	return nil
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			return d
		}
	}
	return def
}
