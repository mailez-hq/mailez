package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"gorm.io/gorm"

	_ "mailez/backend/docs"
	"mailez/backend/internal/admin"
	"mailez/backend/internal/ai"
	"mailez/backend/internal/alias"
	"mailez/backend/internal/announcement"
	"mailez/backend/internal/archive"
	"mailez/backend/internal/auth"
	"mailez/backend/internal/calendar"
	"mailez/backend/internal/compose"
	"mailez/backend/internal/contacts"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/dav"
	"mailez/backend/internal/delegation"
	"mailez/backend/internal/dlp"
	"mailez/backend/internal/domain"
	"mailez/backend/internal/fetch"
	"mailez/backend/internal/ldap"
	"mailez/backend/internal/mailbox"
	"mailez/backend/internal/push"
	"mailez/backend/internal/sieve"
	"mailez/backend/internal/stack"
	"mailez/backend/internal/user"
)

// weakSecrets are known placeholder values that must never reach production;
// they ship as defaults or in the example env file and are trivially guessable.
var weakSecrets = map[string]bool{
	"dev-secret-change-me":      true, // core.Config fallback
	"change-me-please-16-bytes": true, // deploy/mailez.env.example
}

// Server bundles the Fiber app and its dependencies.
type Server struct {
	App      *fiber.App
	DB       *gorm.DB
	Redis    *redis.Client
	Cfg      core.Config
	Auth     *auth.Manager
	LDAP     *ldap.Service
	internal *stack.Handler

	bgCtx    context.Context
	bgCancel context.CancelFunc
}

// New builds the Fiber app, applies migrations and starts background workers.
func New(cfg core.Config) *Server {
	if cfg.Env == "production" && weakSecrets[cfg.SecretKey] {
		log.Fatal("refusing to start in production with a placeholder SECRET_KEY; set a strong secret")
	}
	knownEngine := false
	for _, e := range core.SupportedMailEngines {
		if cfg.MailEngine == e {
			knownEngine = true
			break
		}
	}
	if !knownEngine {
		log.Fatalf("unsupported MAILEZ_MAIL_ENGINE %q (supported: %v)", cfg.MailEngine, core.SupportedMailEngines)
	}
	db := connectDB(cfg)
	rdb := connectRedis(cfg)

	app := fiber.New(fiber.Config{
		AppName:      "mailez",
		BodyLimit:    64 * 1024 * 1024, // aligned with the 20MB attachment cap + base64 overhead
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		// WebDAV methods used by the built-in CardDAV/CalDAV servers; Fiber
		// rejects unknown verbs unless they are registered here.
		RequestMethods: append(append([]string{}, fiber.DefaultMethods...), "PROPFIND", "REPORT", "MKCOL"),
		// Only trust X-Forwarded-For from the stack's own gateway subnet;
		// a client-spoofed XFF must not control c.IP() (login rate limiting).
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{cfg.Subnet},
	})
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${status} ${method} ${path} ${latency} ${ip}\n",
		Next: func(c *fiber.Ctx) bool {
			p := c.Path()
			return p == "/health" || p == "/metrics" || p == "/api/v1/health"
		},
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Request-ID",
	}))
	app.Use(metricsMiddleware)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	s := &Server{App: app, DB: db, Redis: rdb, Cfg: cfg, bgCtx: bgCtx, bgCancel: bgCancel}
	s.Auth = auth.NewManager(db, newStore(rdb, cfg.Env), "mailez_session", time.Duration(cfg.SessionLifetime)*time.Second)
	s.Auth.SetCookieSecure(cfg.CookieSecure)
	s.Auth.SetLoginLimits(cfg.LoginRateLimit, cfg.LoginFailLimit)
	// AD/LDAP directory integration: shared by login, mail-proxy auth and the
	// organization address book sync.
	ldapSvc := ldap.New(db, cfg.SecretKey)
	s.Auth.LDAP = ldapSvc
	s.LDAP = ldapSvc
	s.internal = stack.New(db, s.Auth, cfg, rdb, ldapSvc)
	s.routes()

	// External mailbox poller (fetchmail equivalent).
	fetcher := fetch.New(db, cfg.MailMtaAddr, cfg.SecretKey, cfg.FetchInsecure, time.Duration(cfg.FetchInterval)*time.Second)
	go fetcher.Run(bgCtx)
	// Send-undo queue: delivers parked messages once their window elapses.
	dlpSvc := dlp.New(&core.App{DB: db, Auth: s.Auth, Cfg: cfg})
	go compose.NewOutboxWorker(db, cfg.MailMtaAddr, cfg.SecretKey, dlpSvc).Run(bgCtx)
	go dlpSvc.RunExpiry(bgCtx)
	// Organization address book refresh (AD/LDAP) when directory sync is on.
	go ldap.RunSyncWorker(bgCtx, db, cfg.SecretKey)
	// Compliance archive retention: purge messages past their policy deadline.
	go archive.New(&core.App{DB: db, Auth: s.Auth, Cfg: cfg}).RunRetention(bgCtx)
	// Push notifier (new-mail notifications for subscribed clients).
	if cfg.PushInterval > 0 {
		notifier := push.NewNotifier(db, cfg)
		go notifier.Run(bgCtx)
	}
	if cfg.MetricsAddr != "" {
		startMetricsServer(cfg.MetricsAddr)
	}
	return s
}

// Shutdown cancels background workers and gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.bgCancel()
	return s.App.ShutdownWithContext(ctx)
}

// startMetricsServer exposes Prometheus metrics on a dedicated port, keeping
// the public API surface free of operational endpoints.
func startMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("metrics listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()
}

// newStore prefers Redis and falls back to memory for local dev.
func newStore(rdb *redis.Client, env string) auth.Store {
	err := rdb.Ping(context.Background()).Err()
	if err == nil {
		return auth.NewRedisStore(rdb)
	}
	if env == "production" {
		log.Printf("WARNING: redis unavailable (%v); session store degraded to in-memory. Sessions are lost on restart and rate limits reset — fix redis before relying on this deployment", err)
	} else {
		log.Printf("redis unavailable (%v); using in-memory session store for local dev", err)
	}
	return auth.NewMemoryStore()
}

func (s *Server) routes() {
	// Swagger UI + spec (docs generated by `swag init`).
	s.App.Get("/swagger/*", fiberSwagger.WrapHandler)

	v1 := s.App.Group("/api/v1")
	v1.Get("/health", s.health)
	s.Auth.RegisterSSO(v1)

	app := core.New(s.DB, s.Auth, s.Cfg)
	app.LDAP = s.LDAP
	aiMgr := ai.New(s.DB, s.Cfg)
	user.RegisterPublic(v1, app)
	authed := v1.Group("", app.RequireAuth, app.Audit)

	user.New(app).Register(authed)
	domain.New(app).Register(authed)
	alias.New(app).Register(authed)
	mailbox.New(app).Register(authed)
	compose.New(app).Register(authed)
	delegation.New(app).Register(authed)
	contacts.New(app).Register(authed)
	sieve.New(app).Register(authed)
	admin.New(app).Register(authed)
	announcement.New(app).Register(authed)
	calendar.New(app).Register(authed)
	archive.New(app).Register(authed)
	dlp.New(app).Register(authed)
	fetch.RegisterAPI(authed, app)
	ai.RegisterAPI(authed, app, aiMgr)
	push.RegisterAPI(authed, app)

	stackGroup := s.App.Group("/stack")
	s.internal.Register(stackGroup)
	archive.New(app).RegisterStack(stackGroup)
	dlp.New(app).RegisterStack(stackGroup)

	// Built-in CardDAV/CalDAV servers (phone/desktop sync over Basic Auth)
	// plus the RFC 6764 well-known discovery redirects.
	dav.New(s.DB).Register(s.App.Group("/dav"))
	s.App.Get("/.well-known/carddav", redirectDAV)
	s.App.Get("/.well-known/caldav", redirectDAV)
}

// redirectDAV points DAV discovery clients at the server root.
func redirectDAV(c *fiber.Ctx) error {
	return c.Redirect("/dav/", fiber.StatusMovedPermanently)
}

func (s *Server) health(c *fiber.Ctx) error {
	sqlDB, _ := s.DB.DB()
	if err := sqlDB.Ping(); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "db": "unavailable"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func connectDB(cfg core.Config) *gorm.DB {
	db, err := core.OpenDB(cfg.DBDriver, cfg.DBDSN, cfg.LogLevel)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := models.Migrate(db); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	return db
}

func connectRedis(cfg core.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		// Redis is not critical for the scaffold; log and continue.
		log.Printf("redis connect: %v", err)
	}
	return rdb
}
