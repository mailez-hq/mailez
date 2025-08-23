package auth

import (
	"context"
	"time"
)

// checkLoginAttempt enforces a per-IP attempt limit over a fixed window so
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

// incr bumps a counter key atomically (Store.Incr) and returns its new value.
// A store error fails open (0), matching the pre-existing behavior.
func (m *Manager) incr(ctx context.Context, key string, ttl time.Duration) int {
	n, err := m.Store.Incr(ctx, key, ttl)
	if err != nil {
		return 0
	}
	return n
}
