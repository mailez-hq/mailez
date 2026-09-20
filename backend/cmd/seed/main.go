package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

// seed creates a dev admin user and domain. For local development only.
func main() {
	db, err := core.OpenDB(driver(), dsn(), logLevel())
	if err != nil {
		log.Fatal(err)
	}
	if err := models.Migrate(db); err != nil {
		log.Fatal(err)
	}

	adminDomain := envOr("MAILEZ_DOMAIN", "example.com")
	adminEmail := envOr("MAILEZ_ADMIN_EMAIL", "admin@"+adminDomain)
	adminPassword := envOr("MAILEZ_ADMIN_PASSWORD", "MailezDemo2026!")
	if adminPassword == "MailezDemo2026!" {
		log.Println("WARNING: using the demo admin password; set MAILEZ_ADMIN_PASSWORD for anything but local dev")
	}
	localpart, err := localpartFor(adminEmail, adminDomain)
	if err != nil {
		log.Fatal(err)
	}
	hash, err := password.Hash(adminPassword)
	if err != nil {
		log.Fatal(err)
	}

	domain := models.Domain{Name: adminDomain, MaxUsers: -1, MaxAliases: -1}
	if err := db.FirstOrCreate(&domain, "name = ?", adminDomain).Error; err != nil {
		log.Fatal(err)
	}

	user := models.User{
		Email:       adminEmail,
		Localpart:   localpart,
		DomainName:  adminDomain,
		Password:    hash,
		GlobalAdmin: true,
	}
	if err := db.FirstOrCreate(&user, "email = ?", adminEmail).Error; err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded %s", adminEmail)
}

// localpartFor takes the local part of the admin address. The address has to
// live in MAILEZ_DOMAIN: deriving the local part by slicing on the domain
// length silently produced a wrong address for any other domain and panicked
// once the two strings were the same length.
func localpartFor(email, domain string) (string, error) {
	localpart, emailDomain, ok := strings.Cut(email, "@")
	if !ok || localpart == "" {
		return "", fmt.Errorf("MAILEZ_ADMIN_EMAIL %q is not an email address", email)
	}
	if !strings.EqualFold(emailDomain, domain) {
		return "", fmt.Errorf("MAILEZ_ADMIN_EMAIL %q is not in MAILEZ_DOMAIN %q", email, domain)
	}
	return localpart, nil
}

func dsn() string {
	if v := os.Getenv("DB_DSN"); v != "" {
		return v
	}
	return "mailez.db"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func driver() string {
	return envOr("DB_DRIVER", "sqlite")
}

func logLevel() string {
	return envOr("LOG_LEVEL", "warn")
}
