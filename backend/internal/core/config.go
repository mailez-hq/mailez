package core

import (
	"os"
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
	DovecotAddress     string
	PostfixAddress     string
	MailEngine         string
	RecipientDelimiter string
	Subnet             string
	Domain             string
	Hostname           string
	MailImapAddr       string
	MailSmtpAddr       string
	MailSieveAddr      string
	// MailMtaAddr is the host:port used by background workers (outbox
	// delivery, external fetch poller) to submit mail to the local MTA.
	// Defaults to PostfixAddress:25 (resolves inside the docker network);
	// host-run dev backends must point it at a host-reachable mapping
	// (e.g. 127.0.0.1:25).
	MailMtaAddr        string
	// UploadDir is where the large-attachment relay (超大附件) stores files.
	UploadDir string
	// DriveBackend selects the cloud drive blob backend: "local" (default)
	// or "minio". MinIO settings reuse the MAILEZINE_S3_* variables so the
	// existing minio service in the compose profile works out of the box.
	DriveBackend   string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool
	AIProvider         string
	AIBaseURL          string
	AIAPIKey           string
	AIModel            string
	MessageRateLimit   int // outbound messages per user per hour
	FetchInterval      int // seconds between external mailbox polls
	PushInterval       int // seconds between push notifier polls (0 disables)
	LoginRateLimit     int // login attempts per IP per window
	LoginFailLimit     int // failed logins per email before lockout
	CORSOrigins        string
	LogLevel           string
	FetchInsecure      bool // skip TLS verification for external fetch (opt-out)
	DkimSelector       string
	LicenseFile        string
	License            string
	LicenseRequired    bool
	// KVBackend is the engine storage KV backend ("pebble" | "tidb"). It is
	// reported on the admin overview; the community (postdove) engine stores
	// mail in Maildir instead and leaves this empty.
	KVBackend string
	// BlobBackend is the message-body blob store: "minio" when an S3/MinIO
	// endpoint is configured, otherwise "local".
	BlobBackend string
}

// SupportedMailEngines are the mail engines the control plane can drive.
// The engine selects which adapter consumes the directory contract; the
// postdove engine is the only implementation today.
var SupportedMailEngines = []string{"postdove"}

// Load reads configuration from the environment.
func Load() Config {
	cfg := Config{
		Port:               env("MAILEZ_PORT", "8081"),
		Env:                env("MAILEZ_ENV", "development"),
		MetricsAddr:        env("MAILEZ_METRICS_ADDR", ":9090"),
		DBDriver:           env("DB_DRIVER", "sqlite"),
		DBDSN:              env("DB_DSN", "mailez.db"),
		RedisAddr:          env("REDIS_ADDR", "localhost:6379"),
		SecretKey:          env("MAILEZ_SECRET_KEY", "dev-secret-change-me"),
		CookieSecure:       envBool("MAILEZ_COOKIE_SECURE", false),
		SessionLifetime:    envInt("SESSION_LIFETIME", 3600),
		DovecotAddress:     env("DOVECOT_ADDRESS", "dovecot"),
		PostfixAddress:     env("POSTFIX_ADDRESS", "postfix"),
		MailEngine:         env("MAILEZ_MAIL_ENGINE", "postdove"),
		RecipientDelimiter: env("MAILEZ_RECIPIENT_DELIMITER", ""),
		Subnet:             env("MAILEZ_SUBNET", "192.168.206.0/24"),
		Domain:             env("MAILEZ_DOMAIN", "example.com"),
		Hostname:           env("MAILEZ_HOSTNAME", "localhost"),
		MailImapAddr:       env("MAIL_IMAP_ADDR", "gateway:1143"),
		MailSmtpAddr:       env("MAIL_SMTP_ADDR", "gateway:1587"),
		MailSieveAddr:      env("MAIL_SIEVE_ADDR", "gateway:11490"),
		MailMtaAddr:        env("MAIL_MTA_ADDR", ""),
		UploadDir:          env("MAILEZ_UPLOAD_DIR", "uploads"),
		DriveBackend:       env("MAILEZ_DRIVE_BACKEND", "local"),
		MinioEndpoint:      env("MAILEZINE_S3_ENDPOINT", "minio:9000"),
		MinioAccessKey:     env("MAILEZINE_S3_ACCESS_KEY", ""),
		MinioSecretKey:     env("MAILEZINE_S3_SECRET_KEY", ""),
		MinioBucket:        env("MAILEZINE_S3_BUCKET", "mailezine"),
		MinioUseSSL:        envBool("MAILEZINE_S3_USE_SSL", false),
		AIProvider:         env("AI_PROVIDER", "none"),
		AIBaseURL:          env("AI_BASE_URL", "https://api.openai.com/v1"),
		AIAPIKey:           env("AI_API_KEY", ""),
		AIModel:            env("AI_MODEL", "gpt-4o-mini"),
		MessageRateLimit:   envInt("MAILEZ_MESSAGE_RATELIMIT", 200),
		FetchInterval:      envInt("FETCH_INTERVAL", 300),
		PushInterval:       envInt("PUSH_INTERVAL", 60),
		LoginRateLimit:     envInt("LOGIN_RATELIMIT", 30),
		LoginFailLimit:     envInt("LOGIN_FAIL_LIMIT", 10),
		CORSOrigins:        env("CORS_ORIGINS", "http://localhost:3000,http://localhost:3001"),
		LogLevel:           env("LOG_LEVEL", "info"),
		FetchInsecure:      envBool("FETCH_INSECURE", false),
		DkimSelector:       env("MAILEZ_DKIM_SELECTOR", "dkim"),
		StackSecret:        env("MAILEZ_STACK_SECRET", ""),
		LicenseFile:        env("MAILEZ_LICENSE_FILE", ""),
		License:            env("MAILEZ_LICENSE", ""),
		LicenseRequired:    envBool("MAILEZ_LICENSE_REQUIRED", false),
		KVBackend:          env("MAILEZINE_STORAGE_BACKEND", ""),
	}
	if os.Getenv("MAILEZINE_S3_ENDPOINT") != "" {
		cfg.BlobBackend = "minio"
	} else {
		cfg.BlobBackend = "local"
	}
	if cfg.MailMtaAddr == "" {
		cfg.MailMtaAddr = cfg.PostfixAddress + ":25"
	}
	return cfg
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
