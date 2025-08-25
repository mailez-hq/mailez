package main

import (
	"log"
	"os"

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

	adminEmail := envOr("MAILEZ_ADMIN_EMAIL", "admin@example.com")
	adminDomain := envOr("MAILEZ_DOMAIN", "example.com")
	adminPassword := envOr("MAILEZ_ADMIN_PASSWORD", "MailezDemo2026!")
	if adminPassword == "MailezDemo2026!" {
		log.Println("WARNING: using the demo admin password; set MAILEZ_ADMIN_PASSWORD for anything but local dev")
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
		Localpart:   adminEmail[:len(adminEmail)-len(adminDomain)-1],
		DomainName:  adminDomain,
		Password:    hash,
		GlobalAdmin: true,
	}
	if err := db.FirstOrCreate(&user, "email = ?", adminEmail).Error; err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded %s", adminEmail)
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
