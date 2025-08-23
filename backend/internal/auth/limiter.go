package auth

import (
	"context"
	"strconv"
	"time"
)

// checkLoginAttempt enforces a per-IP attempt limit over a sliding window so
// the web login endpoint cannot be brute-forced (the SMTP path has its own
// limiter in the stack package).
func (m *Manager) checkLoginAttempt(ctx context.Context, ip string) bool {
	if m.loginPerIP <= 0 {
		return true
	}
	return m.incr(ctx, "mailez:login:ip:"+ip, m.loginWindow) <= m.loginPerIP
}

// loginFailed records a failed login per email; it reports true once the
// failure limit is exceeded so the account is temporarily locked out.
func (m *Manager) loginFailed(ctx context.Context, email string) bool {
	if m.loginFail <= 0 {
		return false
	}
	return m.incr(ctx, "mailez:login:fail:"+email, m.loginWindow) > m.loginFail
}

// loginSucceeded clears the per-email failure counter.
func (m *Manager) loginSucceeded(ctx context.Context, email string) {
	_ = m.Store.Delete(ctx, "mailez:login:fail:"+email)
}

// incr bumps a counter key and returns its new value.
func (m *Manager) incr(ctx context.Context, key string, ttl time.Duration) int {
	v, ok, err := m.Store.Get(ctx, key)
	if err != nil {
		return 0
	}
	n := 0
	if ok {
		n, _ = strconv.Atoi(v)
	}
	n++
	_ = m.Store.Set(ctx, key, strconv.Itoa(n), ttl)
	return n
}
