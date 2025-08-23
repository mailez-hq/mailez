package config

import (
	"os"
	"strconv"
)

// Config holds all runtime settings, loaded from environment variables in the
// the reference implementation image style. Every field has a sane default for local dev.
type Config struct {
	Port               string
	DBDriver           string
	DBDSN              string
	RedisAddr          string
	SecretKey          string
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
	DkimSelector       string
}

// Load reads configuration from the environment.
func Load() Config {
	return Config{
		Port:               env("MAILEZ_PORT", "8081"),
		DBDriver:           env("DB_DRIVER", "sqlite"),
		DBDSN:              env("DB_DSN", "mailez.db"),
		RedisAddr:          env("REDIS_ADDR", "localhost:6379"),
		SecretKey:          env("SECRET_KEY", "dev-secret-change-me"),
		SessionLifetime:    envInt("SESSION_LIFETIME", 3600),
		MailKeeperAddress:  env("MAIL_KEEPER_ADDRESS", "mail-keeper"),
		MtaAddress:         env("MTA_ADDRESS", "mta"),
		RecipientDelimiter: env("RECIPIENT_DELIMITER", ""),
		Subnet:             env("SUBNET", "192.168.206.0/24"),
		Domain:             env("DOMAIN", "example.com"),
		Hostname:           env("HOSTNAME", "localhost"),
		MailImapAddr:       env("MAIL_IMAP_ADDR", "gateway:10143"),
		MailSmtpAddr:       env("MAIL_SMTP_ADDR", "gateway:10025"),
		MailSieveAddr:      env("MAIL_SIEVE_ADDR", "gateway:4190"),
		AIProvider:         env("AI_PROVIDER", "none"),
		AIBaseURL:          env("AI_BASE_URL", "https://api.openai.com/v1"),
		AIAPIKey:           env("AI_API_KEY", ""),
		AIModel:            env("AI_MODEL", "gpt-4o-mini"),
		MessageRateLimit:   envInt("MESSAGE_RATELIMIT", 200),
		FetchInterval:      envInt("FETCH_INTERVAL", 300),
		DkimSelector:       env("DKIM_SELECTOR", "dkim"),
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
