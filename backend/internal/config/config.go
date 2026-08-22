package config

import (
	"os"
	"strconv"
)

// Config holds all runtime settings, loaded from environment variables
// following the mailu.env style. Every field has a sane default for local dev.
type Config struct {
	Port            string
	DBDriver        string
	DBDSN           string
	RedisAddr       string
	SecretKey       string
	SessionLifetime int // seconds
	ImapAddress     string
	SmtpAddress     string
}

// Load reads configuration from the environment.
func Load() Config {
	return Config{
		Port:            env("MAILESS_PORT", "8081"),
		DBDriver:        env("DB_DRIVER", "sqlite"),
		DBDSN:           env("DB_DSN", "mailess.db"),
		RedisAddr:       env("REDIS_ADDR", "localhost:6379"),
		SecretKey:       env("SECRET_KEY", "dev-secret-change-me"),
		SessionLifetime: envInt("SESSION_LIFETIME", 3600),
		ImapAddress:     env("IMAP_ADDRESS", "imap"),
		SmtpAddress:     env("SMTP_ADDRESS", "smtp"),
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
