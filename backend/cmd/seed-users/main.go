// seed-users bulk-creates bench users (N users on one domain) directly in
// the SQLite database for load testing. All users share one password hash
// (the auth verification path is identical; only hash generation cost is
// avoided). For benchmark environments only.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

func main() {
	dsn := envOr("DB_DSN", "bench.db")
	count := atoiOr(envOr("BENCH_USERS", "10000"), 10000)
	domainName := envOr("MAILEZ_DOMAIN", "example.com")
	userPassword := envOr("BENCH_PASSWORD", "benchpass")

	db, err := gorm.Open(glebarezsqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := models.Migrate(db); err != nil {
		log.Fatal(err)
	}

	domain := models.Domain{Name: domainName, MaxUsers: -1, MaxAliases: -1}
	if err := db.FirstOrCreate(&domain, "name = ?", domainName).Error; err != nil {
		log.Fatal(err)
	}

	hash, err := password.Hash(userPassword)
	if err != nil {
		log.Fatal(err)
	}

	const batch = 500
	users := make([]models.User, 0, batch)
	created := 0
	for i := 1; i <= count; i++ {
		localpart := fmt.Sprintf("u%05d", i)
		users = append(users, models.User{
			Email:      localpart + "@" + domainName,
			Localpart:  localpart,
			DomainName: domainName,
			Password:   hash,
		})
		if len(users) == batch {
			if err := db.CreateInBatches(users, batch).Error; err != nil {
				log.Fatal(err)
			}
			created += len(users)
			users = users[:0]
			fmt.Printf("seeded %d users\r", created)
		}
	}
	if len(users) > 0 {
		if err := db.CreateInBatches(users, len(users)).Error; err != nil {
			log.Fatal(err)
		}
		created += len(users)
	}
	fmt.Printf("\nseeded %d users on %s (password %q)\n", created, domainName, userPassword)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
