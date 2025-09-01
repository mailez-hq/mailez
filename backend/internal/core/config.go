package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all runtime settings, loaded from environment variables.
// Every field has a sane default for local dev.
type Config struct {
	Port               string
	Env                string
	MetricsAddr        string
	DBDriver           string
	DBDSN              string
	RedisAddr          string
	SecretKey          string
	StackSecret        string
	CookieSecure       bool
	SessionLifetime    int // seconds
	MailEngine         string
	RecipientDelimiter string
	Subnet             string
	Domain             string
	Hostname           string
	// WebAuthn (passkey) relying-party configuration. RPID defaults to the
	// primary domain; origins default to https://<hostname>. Deployments
	// serving the console on other schemes/hosts must override both
	// (MAILEZ_WEBAUTHN_RP_ID / MAILEZ_WEBAUTHN_ORIGINS, comma-separated).
	WebAuthnRPID    string
	WebAuthnOrigins []string
	MailImapAddr    string
	MailSmtpAddr    string
	MailSieveAddr   string
	// MailEngineMgmtAddr is the mailezine management API (host:port). When
	// set (with MailEngineMgmtSecret), deleting a user also purges the
	// account's engine-side data via DELETE /v1/accounts/{email} — without
	// the cascade the engine keeps orphaned mailboxes and a re-created
	// same-address account would inherit the previous owner's mail.
	MailEngineMgmtAddr   string
	MailEngineMgmtSecret string
	// MailMtaAddr is the host:port used by background workers (outbox
	// delivery, external fetch poller) to submit mail to the engine's MTA.
	// Defaults to mailezine:25 (resolves inside the docker network);
	// host-run dev backends must point it at a host-reachable mapping
	// (e.g. 127.0.0.1:25).
	MailMtaAddr string
	// UploadDir is where the large-attachment relay (超大附件) stores files.
	UploadDir string
	// DriveBackend selects the cloud drive blob backend: "local" (default)
	// or "minio". MinIO settings reuse the MAILEZINE_S3_* variables so the
	// existing minio service in the compose profile works out of the box.
	DriveBackend     string
	MinioEndpoint    string
	MinioAccessKey   string
	MinioSecretKey   string
	MinioBucket      string
	MinioUseSSL      bool
	AIProvider       string
	AIBaseURL        string
	AIAPIKey         string
	AIModel          string
	MessageRateLimit int // outbound messages per user per hour
	FetchInterval    int // seconds between external mailbox polls
	PushInterval     int // seconds between push notifier polls (0 disables)
	EventsInterval   int // seconds between webmail SSE mailbox checks (0 disables)
	LoginRateLimit   int // login attempts per IP per window
	LoginFailLimit   int // failed logins per email before lockout
	CORSOrigins      string
	LogLevel         string
	FetchInsecure    bool // skip TLS verification for external fetch (opt-out)
	DkimSelector     string
	LicenseFile      string
	License          string
	LicenseRequired  bool
	// Edition is the runtime deployment tier ("community"/"ce" | "ee" | "").
	// It mirrors the frontend MAILEZ_EDITION build marker: community
	// deployments fall back to the community license (not the unlimited
	// dev license) when no enterprise license is mounted.
	Edition string
	// KVBackend is the engine storage KV backend ("pebble" | "tidb"). It is
	// reported on the admin overview; single-node deployments default to
	// pebble with local-FS blobs.
	KVBackend string
	// BlobBackend is the message-body blob store: "minio" when an S3/MinIO
	// endpoint is configured, otherwise "local".
	BlobBackend string
	// ServiceFile / Service carry the annual technical-service certificate
	// (MAILEZ_SERVICE_FILE / MAILEZ_SERVICE). It is engine-independent: both
	// the community and enterprise editions can subscribe to support.
	ServiceFile string
	Service     string
	// OIDC federation (enterprise SSO). When Issuer/ClientID/ClientSecret are
	// all set, the enterprise build mounts /api/v1/sso/oidc/start + /callback
	// and the login pages offer the federated sign-in button; the community
	// build ignores the settings entirely.
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	// OIDCRedirectURL is the redirect_uri registered at the identity
	// provider. When empty it is derived from the public hostname as
	// https://<hostname>/api/v1/sso/oidc/callback.
	OIDCRedirectURL string
}

// SupportedMailEngines are the mail engines the control plane can drive.
// mailezine is the only engine; the identifier switches edition semantics
// (license capacity) and the overview label.
var SupportedMailEngines = []string{"mailezine"}

// Load reads configuration from the environment.
func Load() Config {
	cfg := Config{
		Port:                 env("MAILEZ_PORT", "8081"),
		Env:                  env("MAILEZ_ENV", "development"),
		MetricsAddr:          env("MAILEZ_METRICS_ADDR", "127.0.0.1:9090"),
		DBDriver:             env("DB_DRIVER", "sqlite"),
		DBDSN:                env("DB_DSN", "mailez.db"),
		RedisAddr:            env("REDIS_ADDR", "localhost:6379"),
		SecretKey:            env("MAILEZ_SECRET_KEY", "dev-secret-change-me"),
		CookieSecure:         envBool("MAILEZ_COOKIE_SECURE", false),
		SessionLifetime:      envInt("SESSION_LIFETIME", 3600),
		MailEngine:           env("MAILEZ_MAIL_ENGINE", "mailezine"),
		RecipientDelimiter:   env("MAILEZ_RECIPIENT_DELIMITER", ""),
		Subnet:               env("MAILEZ_SUBNET", "192.168.206.0/24"),
		Domain:               env("MAILEZ_DOMAIN", "example.com"),
		Hostname:             env("MAILEZ_HOSTNAME", "localhost"),
		WebAuthnRPID:         env("MAILEZ_WEBAUTHN_RP_ID", env("MAILEZ_DOMAIN", "example.com")),
		WebAuthnOrigins:      splitCSV(env("MAILEZ_WEBAUTHN_ORIGINS", "https://"+env("MAILEZ_HOSTNAME", "localhost"))),
		MailImapAddr:         env("MAIL_IMAP_ADDR", "mailezine:143"),
		MailSmtpAddr:         env("MAIL_SMTP_ADDR", "mailezine:1587"),
		MailSieveAddr:        env("MAIL_SIEVE_ADDR", "mailezine:4190"),
		MailEngineMgmtAddr:   env("MAIL_ENGINE_MGMT_ADDR", ""),
		MailEngineMgmtSecret: env("MAIL_ENGINE_MGMT_SECRET", ""),
		MailMtaAddr:          env("MAIL_MTA_ADDR", ""),
		UploadDir:            env("MAILEZ_UPLOAD_DIR", defaultUploadDir()),
		DriveBackend:         env("MAILEZ_DRIVE_BACKEND", ""),
		MinioEndpoint:        env("MAILEZINE_S3_ENDPOINT", ""),
		MinioAccessKey:       env("MAILEZINE_S3_ACCESS_KEY", ""),
		MinioSecretKey:       env("MAILEZINE_S3_SECRET_KEY", ""),
		MinioBucket:          env("MAILEZINE_S3_BUCKET", "mailezine"),
		MinioUseSSL:          envBool("MAILEZINE_S3_USE_SSL", false),
		AIProvider:           env("AI_PROVIDER", "none"),
		AIBaseURL:            env("AI_BASE_URL", "https://api.openai.com/v1"),
		AIAPIKey:             env("AI_API_KEY", ""),
		AIModel:              env("AI_MODEL", "gpt-4o-mini"),
		MessageRateLimit:     envInt("MAILEZ_MESSAGE_RATELIMIT", 200),
		FetchInterval:        envInt("FETCH_INTERVAL", 300),
		PushInterval:         envInt("PUSH_INTERVAL", 60),
		EventsInterval:       envInt("MAILEZ_EVENTS_INTERVAL", 20),
		LoginRateLimit:       envInt("LOGIN_RATELIMIT", 30),
		LoginFailLimit:       envInt("LOGIN_FAIL_LIMIT", 10),
		CORSOrigins:          env("CORS_ORIGINS", "http://localhost:3000,http://localhost:3001"),
		LogLevel:             env("LOG_LEVEL", "info"),
		FetchInsecure:        envBool("FETCH_INSECURE", false),
		DkimSelector:         env("MAILEZ_DKIM_SELECTOR", "dkim"),
		StackSecret:          env("MAILEZ_STACK_SECRET", ""),
		LicenseFile:          env("MAILEZ_LICENSE_FILE", ""),
		License:              env("MAILEZ_LICENSE", ""),
		LicenseRequired:      envBool("MAILEZ_LICENSE_REQUIRED", false),
		// Edition mirrors the frontend MAILEZ_EDITION build marker at
		// runtime: "community"/"ce" tells the backend its no-license
		// fallback is a community deployment, not a developer checkout.
		Edition:          env("MAILEZ_EDITION", ""),
		ServiceFile:      env("MAILEZ_SERVICE_FILE", ""),
		Service:          env("MAILEZ_SERVICE", ""),
		OIDCIssuer:       env("MAILEZ_OIDC_ISSUER", ""),
		OIDCClientID:     env("MAILEZ_OIDC_CLIENT_ID", ""),
		OIDCClientSecret: env("MAILEZ_OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURL:  env("MAILEZ_OIDC_REDIRECT_URL", ""),
	}
	// Distributed deployments opt into TiDB KV + MinIO/S3 blobs by setting
	// MAILEZINE_STORAGE_BACKEND / MAILEZINE_S3_* explicitly (the enterprise
	// compose does). Single-node community/dev deployments keep the
	// pebble/local-FS defaults — the engine has no TiDB or MinIO to report.
	cfg.KVBackend = env("MAILEZINE_STORAGE_BACKEND", "")
	if cfg.KVBackend == "" && cfg.MailEngine == "mailezine" {
		cfg.KVBackend = "pebble"
	}
	if os.Getenv("MAILEZINE_S3_ENDPOINT") != "" {
		cfg.BlobBackend = "minio"
	} else {
		cfg.BlobBackend = "local"
	}
	// The drive shares the blob backend decision: a deployment that points
	// MAILEZINE_S3_* at MinIO (the enterprise compose does) gets MinIO-backed
	// drive files instead of a CWD-relative "uploads" directory that is not
	// writable in containers. An explicit MAILEZ_DRIVE_BACKEND still wins.
	if cfg.DriveBackend == "" {
		if os.Getenv("MAILEZINE_S3_ENDPOINT") != "" {
			cfg.DriveBackend = "minio"
		} else {
			cfg.DriveBackend = "local"
		}
	}
	if cfg.MailMtaAddr == "" {
		cfg.MailMtaAddr = "mailezine:25"
	}
	return cfg
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// defaultUploadDir keeps the local blob store writable in containers: when
// the database is a sqlite file (the community/dev default), blobs default
// next to it (e.g. /data/mailez.db → /data/uploads) instead of a
// CWD-relative "uploads" that the container filesystem may not allow.
// Non-sqlite deployments (mysql DSN) keep the historical relative default.
func defaultUploadDir() string {
	if env("DB_DRIVER", "sqlite") == "sqlite" {
		if dsn := env("DB_DSN", "mailez.db"); !strings.Contains(dsn, ":") || strings.ContainsRune(dsn, filepath.Separator) || strings.ContainsRune(dsn, '/') {
			if dir := filepath.Dir(dsn); dir != "" && dir != "." {
				return filepath.Join(dir, "uploads")
			}
		}
	}
	return "uploads"
}

// splitCSV parses a comma-separated environment value into a trimmed,
// non-empty string slice.
func splitCSV(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}
