package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
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
	"mailez/backend/internal/alias"
	"mailez/backend/internal/auth"
	"mailez/backend/internal/authcache"
	"mailez/backend/internal/calendar"
	"mailez/backend/internal/compose"
	"mailez/backend/internal/contacts"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/dav"
	"mailez/backend/internal/domain"
	"mailez/backend/internal/drive"
	"mailez/backend/internal/fetch"
	"mailez/backend/internal/invite"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/mailbox"
	"mailez/backend/internal/push"
	"mailez/backend/internal/sieve"
	"mailez/backend/internal/stack"
	"mailez/backend/internal/uploads"
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
	LDAP     core.DirectorySync
	internal *stack.Handler
	// events fans mailbox-change events to open webmail SSE connections.
	events *push.Hub
	// basicAuthCache memoizes Basic-auth credential verification for the
	// protocol endpoints that re-authenticate on every request.
	basicAuthCache *authcache.Cache

	bgCtx    context.Context
	bgCancel context.CancelFunc
}

// New builds the Fiber app, applies migrations and starts background workers.
func New(cfg core.Config) *Server {
	if cfg.Env == "production" && weakSecrets[cfg.SecretKey] {
		log.Fatal("refusing to start in production with a placeholder SECRET_KEY; set a strong secret")
	}
	// An empty stack secret would leave /stack unauthenticated (the
	// internal API hands out decrypted fetch passwords and DKIM keys).
	if cfg.Env == "production" && strings.TrimSpace(cfg.StackSecret) == "" {
		log.Fatal("refusing to start in production without MAILEZ_STACK_SECRET; the internal /stack API would be unauthenticated")
	}
	if !slices.Contains(core.SupportedMailEngines, cfg.MailEngine) {
		log.Fatalf("unsupported MAILEZ_MAIL_ENGINE %q (supported: %v)", cfg.MailEngine, core.SupportedMailEngines)
	}
	db := connectDB(cfg)
	rdb := connectRedis(cfg)

	app := fiber.New(fiber.Config{
		AppName:     "mailez",
		BodyLimit:   64 * 1024 * 1024, // aligned with the 20MB attachment cap + base64 overhead
		ReadTimeout: 30 * time.Second,
		// No write timeout: long-lived responses (the AI compose SSE stream
		// and the /events mailbox push) stay open for hours. fasthttp applies
		// WriteTimeout as one absolute deadline for the whole response write,
		// which would kill an SSE connection at 60s. Slow clients are bounded
		// by ReadTimeout and IdleTimeout instead.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
		// WebDAV methods used by the built-in CardDAV/CalDAV servers; Fiber
		// rejects unknown verbs unless they are registered here.
		RequestMethods: append(append([]string{}, fiber.DefaultMethods...), "PROPFIND", "REPORT", "MKCOL"),
		// Only trust X-Forwarded-For from the stack's own gateway subnet;
		// a client-spoofed XFF must not control c.IP() (login rate limiting).
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{cfg.Subnet},
		// Single error exit point: known *fiber.Error values surface their
		// (developer-authored) message as JSON; anything else — unexpected
		// returns from handlers — is logged server-side and reported
		// generically so internals (gorm/redis/driver text) never leak.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var fe *fiber.Error
			if errors.As(err, &fe) {
				return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
			}
			log.Printf("api error (%d %s): %v", code, c.Path(), err)
			return c.Status(code).JSON(fiber.Map{"error": "internal error"})
		},
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
	s.events = push.NewHub()
	s.Auth = auth.NewManager(db, newStore(rdb, cfg.Env), "mailez_session", time.Duration(cfg.SessionLifetime)*time.Second)
	s.Auth.SetCookieSecure(cfg.CookieSecure)
	s.Auth.SetLoginLimits(cfg.LoginRateLimit, cfg.LoginFailLimit)
	// Security alert on login from a new IP/device: mail the account owner.
	s.Auth.NotifyLogin = func(email, ip, ua string) {
		if ua == "" {
			ua = "未知设备"
		}
		text := fmt.Sprintf("您的账号于 %s 从 IP %s 登录（设备：%s）。\n如非本人操作，请立即修改密码。\n",
			time.Now().Format("2006-01-02 15:04:05"), ip, ua)
		raw := mail.BuildMessage(email, []string{email}, nil, "安全提醒：新设备登录", text, "", nil)
		if err := mail.SubmitMTA(cfg.MailMtaAddr, email, []string{email}, raw); err != nil {
			log.Printf("login alert to %s: %v", email, err)
		}
	}
	// AD/LDAP directory integration (login fallback, mail-proxy auth,
	// address-book sync) is an enterprise capability: the seam returns nil
	// in the community build.
	dirSync := eeNewDirectorySync(db, cfg.SecretKey)
	s.Auth.LDAP = dirSync
	s.LDAP = dirSync
	basicAuthCache := authcache.New(basicAuthCacheTTL())
	s.basicAuthCache = basicAuthCache
	// Push notifier (new-mail notifications for subscribed clients) and the
	// webmail SSE watcher are created before the stack handler so engine
	// delivery receipts can kick either one immediately.
	notifier := push.NewNotifier(db, cfg)
	eventWatcher := push.NewEventWatcher(cfg, s.events)
	s.internal = stack.New(db, s.Auth, cfg, rdb, dirSync, basicAuthCache)
	s.internal.Notifier = notifier
	s.internal.EventWatcher = eventWatcher
	s.routes()

	// External mailbox poller (fetchmail equivalent).
	fetcher := fetch.New(db, cfg.MailMtaAddr, cfg.SecretKey, cfg.FetchInsecure, time.Duration(cfg.FetchInterval)*time.Second)
	go fetcher.Run(bgCtx)
	// Send-undo queue: delivers parked messages once their window elapses.
	// The outbound DLP scanner (content filter/审批) is an enterprise
	// capability: the seam returns nil in the community build.
	dlpScanner := eeStartComplianceWorkers(db, s.Auth, cfg, bgCtx)
	go compose.NewOutboxWorker(db, cfg.MailMtaAddr, cfg.SecretKey, dlpScanner, mail.New(cfg.MailImapAddr, "", "").SetInsecureTLS(cfg.FetchInsecure)).Run(bgCtx)
	// Calendar event reminders: mail the owner when start - reminder arrives.
	go calendar.NewReminderWorker(db, cfg).Run(bgCtx)
	// Large-attachment relay cleanup: delete expired uploads.
	go uploads.New(db, cfg).RunCleanup(bgCtx)
	// Push notifier (new-mail notifications for subscribed clients).
	if cfg.PushInterval > 0 {
		go notifier.Run(bgCtx)
	}
	// Webmail mailbox-change stream: polls connected users' folders and pushes
	// a "mail" event down their SSE connections when new mail arrives.
	if cfg.EventsInterval > 0 {
		go eventWatcher.Run(bgCtx)
	}
	if cfg.MetricsAddr != "" {
		startMetricsServer(cfg.MetricsAddr)
	}
	return s
}

// basicAuthCacheTTL resolves the memoization window for HTTP Basic
// credential checks (CalDAV/CardDAV/ActiveSync/webdav), in seconds.
func basicAuthCacheTTL() time.Duration {
	if v := os.Getenv("MAILEZ_BASIC_AUTH_CACHE_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 10 * time.Minute
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

// publicBranding is the white-label brand shape served by the public
// /server/settings endpoint. Zero values mean "use the built-in Mailez
// brand"; filling it is edition-seamed (eePublicBranding).
type publicBranding struct {
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Tagline   string `json:"tagline"`
	Feature1  string `json:"feature1"`
	Feature2  string `json:"feature2"`
	Feature3  string `json:"feature3"`
	LogoURL   string `json:"logo_url"`
	HeroURL   string `json:"hero_url"`
	Copyright string `json:"copyright"`
	Contact   string `json:"contact"`
}

func (s *Server) routes() {
	// Swagger UI + spec (docs generated by `swag init`).
	s.App.Get("/swagger/*", fiberSwagger.WrapHandler)

	v1 := s.App.Group("/api/v1")
	v1.Get("/health", s.health)
	// Public mail-server settings for the login-page client-setup hint.
	// All services share the configured public hostname; the port matrix is
	// the gateway/engine mail port contract (plain + implicit TLS).
	v1.Get("/server/settings", func(c *fiber.Ctx) error {
		var domains []string
		s.DB.Model(&models.Domain{}).Order("name asc").Pluck("name", &domains)
		// White-label branding is edition-seamed: EE fills it from the
		// admin configuration, CE keeps the built-in Mailez brand (zeros).
		brand := publicBranding{}
		eePublicBranding(s, &brand)
		return c.JSON(fiber.Map{
			"hostname":       s.Cfg.Hostname,
			"domain":         s.Cfg.Domain,
			"smtp":           fiber.Map{"plain": 25, "submission": 587, "ssl": 465},
			"pop3":           fiber.Map{"plain": 110, "ssl": 995},
			"imap":           fiber.Map{"plain": 143, "ssl": 993},
			"branding":       brand,
			"domains":        domains,
			"default_domain": s.Cfg.Domain,
			// Federated sign-in is edition-seamed: the community build always
			// reports false so login pages never advertise a missing route.
			"oidc": fiber.Map{"enabled": auth.OIDCEnabled(s.Cfg.OIDCIssuer, s.Cfg.OIDCClientID, s.Cfg.OIDCClientSecret)},
		})
	})
	s.Auth.RegisterSSO(v1)
	// Federated sign-in (OIDC) is an enterprise capability: the community
	// build mounts no routes. The redirect URI defaults to the public
	// hostname when MAILEZ_OIDC_REDIRECT_URL is unset.
	oidcRedirect := s.Cfg.OIDCRedirectURL
	if oidcRedirect == "" {
		oidcRedirect = "https://" + s.Cfg.Hostname + "/api/v1/sso/oidc/callback"
	}
	s.Auth.RegisterOIDC(v1, auth.OIDCConfig{
		Issuer:       s.Cfg.OIDCIssuer,
		ClientID:     s.Cfg.OIDCClientID,
		ClientSecret: s.Cfg.OIDCClientSecret,
		RedirectURL:  oidcRedirect,
	})

	app := core.New(s.DB, s.Auth, s.Cfg)
	// AD/LDAP wiring (app.LDAP + licensed capacity guard) is edition-seamed.
	eeWireAppDirectory(s, app)
	user.RegisterPublic(v1, app)
	// The ICS export is fetched by external calendar clients with only the
	// HMAC feed token, so it must sit outside RequireAuth. Order matters:
	// fiber implements Group(prefix, handlers...) as Use-style middleware on
	// the merged prefix, so every route registered on v1 AFTER the authed
	// group below inherits RequireAuth — public routes must come first
	// (same pattern as /health, /server/settings and user.RegisterPublic).
	calendarHandler := calendar.New(app)
	calendarHandler.RegisterPublic(v1)
	authed := v1.Group("", app.RequireAuth, app.Audit)

	user.New(app).Register(authed)
	domain.New(app).Register(authed)
	alias.New(app).Register(authed)
	mailbox.New(app).Register(authed)
	compose.New(app).Register(authed)
	contacts.New(app).Register(authed)
	sieve.New(app).Register(authed)
	admin.New(app).Register(authed)
	calendarHandler.Register(authed)
	invite.New(app).Register(authed)
	fetch.RegisterAPI(authed, app)
	push.RegisterAPI(authed, app, s.events)
	uploads.New(s.DB, s.Cfg).Register(authed)
	if driveSvc, derr := drive.New(s.DB, s.Cfg); derr == nil {
		driveSvc.Register(authed)
	} else {
		log.Printf("drive: %v", derr)
	}

	stackGroup := s.App.Group("/stack", requireStackSecret(s.Cfg.StackSecret))
	s.internal.Register(stackGroup)

	// Built-in CardDAV/CalDAV servers (phone/desktop sync over Basic Auth)
	// plus the RFC 6764 well-known discovery redirects.
	dav.New(s.DB, s.basicAuthCache).Register(s.App.Group("/dav"))
	s.App.Get("/.well-known/carddav", redirectDAV)
	s.App.Get("/.well-known/caldav", redirectDAV)

	// Enterprise routes (delegation, announcement, archive, DLP, AI,
	// ActiveSync) mount through the edition seam: nothing in CE.
	eeRegisterRoutes(s, app, v1, authed, stackGroup)
}

// requireStackSecret guards the internal /stack API used by mailezine and
// the mail agent. When MAILEZ_STACK_SECRET is empty (local dev) every
// caller is accepted; otherwise the caller must present the shared secret in
// the X-Stack-Secret header (kept in sync with the enterprise agent's
// stackclient.SecretHeader).
func requireStackSecret(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if secret == "" {
			return c.Next()
		}
		got := c.Get("X-Stack-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			return c.SendStatus(fiber.StatusForbidden)
		}
		return c.Next()
	}
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
