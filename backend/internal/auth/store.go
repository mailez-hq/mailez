package auth

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store persists SSO sessions and temporary tokens. Sessions map a session id
// to a user email; tokens map a token to a session id.
type Store interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, bool, error)
	Delete(ctx context.Context, key string) error
	// Incr atomically bumps a counter key and returns the new value. The ttl
	// applies from the first increment (fixed window); implementations must
	// be race-free because counters gate login rate limiting.
	Incr(ctx context.Context, key string, ttl time.Duration) (int, error)
}

// RedisStore backs sessions with Redis.
type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, value, ttl).Err()
}

func (s *RedisStore) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *RedisStore) Delete(ctx context.Context, key string) error {
	return s.rdb.Del(ctx, key).Err()
}

// Incr counts atomically with Redis INCR; the TTL is set once, on the first
// increment, so the window is anchored at the first attempt.
func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int, error) {
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		if err := s.rdb.Expire(ctx, key, ttl).Err(); err != nil {
			return int(n), err
		}
	}
	return int(n), nil
}

// MemoryStore is a dev fallback when Redis is unavailable. Not for production.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]memoryItem
}

type memoryItem struct {
	value string
	exp   time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]memoryItem)}
}

func (s *MemoryStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = memoryItem{value: value, exp: time.Now().Add(ttl)}
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.data[key]
	if !ok {
		return "", false, nil
	}
	if time.Now().After(item.exp) {
		delete(s.data, key)
		return "", false, nil
	}
	return item.value, true, nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

// Incr mirrors RedisStore.Incr under the store lock: the TTL anchors at the
// first increment and expired counters restart at 1.
func (s *MemoryStore) Incr(ctx context.Context, key string, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.data[key]
	if !ok || time.Now().After(item.exp) {
		s.data[key] = memoryItem{value: "1", exp: time.Now().Add(ttl)}
		return 1, nil
	}
	n, _ := strconv.Atoi(item.value)
	n++
	item.value = strconv.Itoa(n)
	s.data[key] = item
	return n, nil
}
