package stack

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/authcache"
	"mailez/backend/internal/core"
	"mailez/backend/internal/push"
)

// GroupResolver expands AD/LDAP distribution-group membership for alias
// resolution. The optional directory-integration module supplies it; nil
// (the default build) skips the LdapGroup branch entirely.
type GroupResolver interface {
	ResolveGroupMembers(ctx context.Context, groupEmail string) ([]string, error)
}

// Handler implements the internal API consumed by nginx (auth_request) and the
// mail images (nginx mail proxy auth). Its response contract must stay
// byte-compatible with the internal API the container agents consume.
type Handler struct {
	DB    *gorm.DB
	Auth  *auth.Manager
	Cfg   core.Config
	Redis *redis.Client
	LDAP  GroupResolver
	srs   *srsCodec
	rate  *rateLimiter
	// authCache memoizes successful HTTP Basic verifications (webdav auth
	// requests from nginx), which would otherwise pay bcrypt per request.
	authCache *authcache.Cache
	// Notifier/EventWatcher receive engine delivery receipts (nil disables
	// the kick; the endpoint still answers 202).
	Notifier     *push.Notifier
	EventWatcher *push.EventWatcher
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg core.Config, rdb *redis.Client, groups GroupResolver, cache *authcache.Cache) *Handler {
	if cache == nil {
		cache = authcache.New(0)
	}
	return &Handler{
		DB:        db,
		Auth:      authMgr,
		Cfg:       cfg,
		Redis:     rdb,
		LDAP:      groups,
		srs:       newSRSCodec(cfg.SecretKey),
		rate:      newRateLimiter(rdb, cfg.MessageRateLimit),
		authCache: cache,
	}
}

// Register mounts the internal endpoints under /internal.
func (h *Handler) Register(r fiber.Router) {
	r.Get("/auth/user", h.authUser)
	r.Get("/auth/admin", h.authAdmin)
	r.Get("/auth/basic", h.authBasic)
	r.Get("/auth/email", h.authEmail)
	h.registerDirectory(r)
	h.registerRspamd(r)
	h.registerFetch(r)
	h.registerNotify(r)
}
