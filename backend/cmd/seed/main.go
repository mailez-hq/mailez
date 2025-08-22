package main

import (
	"log"
	"os"

	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailess/backend/internal/models"
	"mailess/backend/internal/password"
)

// seed creates a dev admin user and domain. For local development only.
func main() {
	db, err := gorm.Open(glebarezsqlite.Open(dsn()), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		log.Fatal(err)
	}

	hash, err := password.Hash("MailuDemo2026!")
	if err != nil {
		log.Fatal(err)
	}

	domain := models.Domain{Name: "example.com", MaxUsers: -1, MaxAliases: -1}
	if err := db.FirstOrCreate(&domain, "name = ?", "example.com").Error; err != nil {
		log.Fatal(err)
	}

	user := models.User{
		Email:       "admin@example.com",
		Localpart:   "admin",
		DomainName:  "example.com",
		Password:    hash,
		GlobalAdmin: true,
	}
	if err := db.FirstOrCreate(&user, "email = ?", "admin@example.com").Error; err != nil {
		log.Fatal(err)
	}
	log.Println("seeded admin@example.com / MailuDemo2026!")
}

func dsn() string {
	if v := os.Getenv("DB_DSN"); v != "" {
		return v
	}
	return "mailess.db"
}
