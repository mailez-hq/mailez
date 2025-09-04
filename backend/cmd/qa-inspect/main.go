// Command qa-inspect prints a read-only snapshot of the local SQLite
// database (first users + totals) for QA triage; it never mutates data.
// Resets and provisioning belong to the seed tooling / admin console.
package main

import (
	"fmt"
	"log"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func main() {
	db, err := core.OpenDB("sqlite", "mailez.db", "warn")
	if err != nil {
		log.Fatal(err)
	}
	var users []models.User
	if err := db.Limit(10).Find(&users).Error; err != nil {
		log.Fatal(err)
	}
	for _, u := range users {
		fmt.Printf("%s | admin=%v enabled=%v len(pw)=%d updated=%s\n", u.Email, u.GlobalAdmin, u.Enabled, len(u.Password), u.UpdatedAt)
	}
	var n int64
	db.Model(&models.User{}).Count(&n)
	fmt.Println("total users:", n)
}
