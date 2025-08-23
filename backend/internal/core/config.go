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
	CookieSecure       bool
	SessionLifetime    int // seconds
	MailKeeperAddress  string
	MtaAddress         string
	RecipientDelimiter string
	Subnet             string
	Domain             string
	Hostname           string
	MailImapAddr       string
	MailSmtpAddr       string
	MailSieveAddr      string
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
}

// Load reads configuration from the environment.
func Load() Config {
	return Config{
		Port:               env("MAILEZ_PORT", "8081"),
		Env:                env("MAILEZ_ENV", "development"),
		MetricsAddr:        env("MAILEZ_METRICS_ADDR", ":9090"),
		DBDriver:           env("DB_DRIVER", "sqlite"),
		DBDSN:              env("DB_DSN", "mailez.db"),
		RedisAddr:          env("REDIS_ADDR", "localhost:6379"),
		SecretKey:          env("MAILEZ_SECRET_KEY", "dev-secret-change-me"),
		CookieSecure:       envBool("MAILEZ_COOKIE_SECURE", false),
		SessionLifetime:    envInt("SESSION_LIFETIME", 3600),
		MailKeeperAddress:  env("MAIL_KEEPER_ADDRESS", "mail-keeper"),
		MtaAddress:         env("MTA_ADDRESS", "mta"),
		RecipientDelimiter: env("MAILEZ_RECIPIENT_DELIMITER", ""),
		Subnet:             env("MAILEZ_SUBNET", "192.168.206.0/24"),
		Domain:             env("MAILEZ_DOMAIN", "example.com"),
		Hostname:           env("MAILEZ_HOSTNAME", "localhost"),
		MailImapAddr:       env("MAIL_IMAP_ADDR", "gateway:1143"),
		MailSmtpAddr:       env("MAIL_SMTP_ADDR", "gateway:1587"),
		MailSieveAddr:      env("MAIL_SIEVE_ADDR", "gateway:4190"),
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
	}
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
