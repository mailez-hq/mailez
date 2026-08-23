package main

import (
	"log"
	"os"

	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

// seed creates a dev admin user and domain. For local development only.
func main() {
	db, err := gorm.Open(glebarezsqlite.Open(dsn()), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
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
