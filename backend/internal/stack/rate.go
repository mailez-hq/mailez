package stack

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// rateLimiter counts hits per key over a sliding window, preferring Redis and
// falling back to an in-memory table for local dev without Redis.
type rateLimiter struct {
	redis   *redis.Client
	limit   int
	window  time.Duration
	mu      sync.Mutex
	mem     map[string]int
	memSeen map[string]time.Time
}

func newRateLimiter(rdb *redis.Client, limit int) *rateLimiter {
	return &rateLimiter{
		redis:   rdb,
		limit:   limit,
		window:  time.Hour,
		mem:     map[string]int{},
		memSeen: map[string]time.Time{},
	}
}

// hit increments the counter for key and reports whether the limit is now
// exceeded.
func (l *rateLimiter) hit(key string) bool {
	if l.limit <= 0 {
		return false
	}
	if l.redis != nil {
		ctx := context.Background()
		k := "rate:" + key
		n, err := l.redis.Incr(ctx, k).Result()
		if err == nil {
			if n == 1 {
				l.redis.Expire(ctx, k, l.window)
			}
			return n > int64(l.limit)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if seen, ok := l.memSeen[key]; ok && now.Sub(seen) >= l.window {
		l.mem[key] = 0
		l.memSeen[key] = now
	}
	if _, ok := l.memSeen[key]; !ok {
		l.memSeen[key] = now
	}
	l.mem[key]++
	return l.mem[key] > l.limit
}
