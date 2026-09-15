// IP ban engine for credential-stuffing defense across every auth surface
// (web login, mail protocol SASL). Failures are counted per source IP over
// a fixed window through the shared session store; crossing the threshold
// bans the IP, with the ban duration escalating on the IP's ban history
// over the last 30 days. Whitelisted networks — loopback, the deployment's
// own addresses and anything configured — are exempt, so health probes and
// monitoring can never ban themselves.
package ban

import (
	"context"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Counter is the atomic windowed counter the failure threshold counts on;
// the shared session store satisfies it.
type Counter interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int, error)
	Delete(ctx context.Context, key string) error
}

// escalations are the ban durations indexed by prior ban count in the last
// 30 days (none, one, two, three or more).
var escalations = []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}

const failKeyPrefix = "ban:fail:"

// Engine records failures and answers the ban check for every auth surface.
// A nil Engine, or one with a zero threshold, is a no-op.
type Engine struct {
	DB    *gorm.DB
	Count Counter
	max   int
	find  time.Duration
	wl    []*net.IPNet
}

// New wires the engine.
func New(db *gorm.DB, counter Counter, cfg core.Config) *Engine {
	e := &Engine{
		DB:    db,
		Count: counter,
		max:   cfg.BanMaxRetry,
		find:  time.Duration(cfg.BanFindTimeSec) * time.Second,
	}
	if e.find <= 0 {
		e.find = 10 * time.Minute
	}
	// Loopback and the deployment's own addresses: probes and monitoring
	// dial the auth path and must never ban themselves.
	e.whitelist("127.0.0.0/8")
	e.whitelist("::1/128")
	if host := cfg.Hostname; host != "" && host != "localhost" && host != "127.0.0.1" {
		if ips, err := net.LookupHost(host); err == nil {
			for _, ip := range ips {
				e.whitelist(ip)
			}
		}
	}
	for _, cidr := range strings.Split(cfg.BanWhitelist, ",") {
		if cidr = strings.TrimSpace(cidr); cidr != "" {
			e.whitelist(cidr)
		}
	}
	return e
}

// whitelist adds a CIDR (or a bare IP, upgraded to /32 or /128).
func (e *Engine) whitelist(cidr string) {
	if !strings.Contains(cidr, "/") {
		if strings.Contains(cidr, ":") {
			cidr += "/128"
		} else {
			cidr += "/32"
		}
	}
	if _, n, err := net.ParseCIDR(cidr); err == nil {
		e.wl = append(e.wl, n)
	}
}

func (e *Engine) whitelisted(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	for _, n := range e.wl {
		if n.Contains(p) {
			return true
		}
	}
	return false
}

// Active reports whether ip is currently banned. Database errors fail open:
// a broken backend must not lock every user out.
func (e *Engine) Active(ctx context.Context, ip string) bool {
	if e == nil || e.max <= 0 || e.Count == nil || e.DB == nil || e.whitelisted(ip) {
		return false
	}
	var n int64
	if err := e.DB.WithContext(ctx).Model(&models.BanRecord{}).
		Where("ip = ? AND lifted_at IS NULL AND until > ?", ip, time.Now()).
		Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// Failure records an authentication failure from ip on surface; when the
// windowed count reaches the threshold the IP is banned.
func (e *Engine) Failure(ctx context.Context, ip, surface string) {
	if e == nil || e.max <= 0 || e.Count == nil || e.DB == nil || e.whitelisted(ip) {
		return
	}
	n, err := e.Count.Incr(ctx, failKeyPrefix+ip, e.find)
	if err != nil || n < e.max {
		return
	}
	e.ban(ctx, ip, surface, n)
}

func (e *Engine) ban(ctx context.Context, ip, surface string, failed int) {
	var prior int64
	if err := e.DB.Model(&models.BanRecord{}).
		Where("ip = ? AND created_at > ?", ip, time.Now().AddDate(0, 0, -30)).
		Count(&prior).Error; err != nil {
		return
	}
	idx := int(prior)
	if idx > len(escalations)-1 {
		idx = len(escalations) - 1
	}
	e.DB.Create(&models.BanRecord{
		IP:      ip,
		Surface: surface,
		Failed:  failed,
		Until:   time.Now().Add(escalations[idx]),
	})
	// The failure window restarts while the ban is in force.
	_ = e.Count.Delete(ctx, failKeyPrefix+ip)
}

// Reset clears the failure window, e.g. after a successful login.
func (e *Engine) Reset(ctx context.Context, ip string) {
	if e == nil || e.Count == nil {
		return
	}
	_ = e.Count.Delete(ctx, failKeyPrefix+ip)
}

// List returns the active bans, newest expiry first.
func (e *Engine) List(ctx context.Context) ([]models.BanRecord, error) {
	var out []models.BanRecord
	err := e.DB.WithContext(ctx).
		Where("lifted_at IS NULL AND until > ?", time.Now()).
		Order("until desc").Limit(200).Find(&out).Error
	return out, err
}

// Lift ends a ban early; the row stays as history.
func (e *Engine) Lift(ctx context.Context, id uint) error {
	var rec models.BanRecord
	if err := e.DB.WithContext(ctx).First(&rec, id).Error; err != nil {
		return err
	}
	now := time.Now()
	if err := e.DB.Model(&rec).Updates(map[string]any{"lifted_at": now, "until": now}).Error; err != nil {
		return err
	}
	_ = e.Count.Delete(ctx, failKeyPrefix+rec.IP)
	return nil
}

// RecentCount reports how many bans were created since since (health feed).
func (e *Engine) RecentCount(ctx context.Context, since time.Time) (int64, error) {
	var n int64
	err := e.DB.WithContext(ctx).Model(&models.BanRecord{}).
		Where("created_at > ?", since).Count(&n).Error
	return n, err
}
