package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

func TestRememberLoginIPAlertOnNewIP(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{Email: "alice@example.com", Localpart: "alice", DomainName: "example.com", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	m := &Manager{DB: db}
	ctx := context.Background()
	if !m.rememberLoginIP(ctx, "alice@example.com", "1.2.3.4") {
		t.Fatal("first login from a new IP must alert")
	}
	if m.rememberLoginIP(ctx, "alice@example.com", "1.2.3.4") {
		t.Fatal("same IP must not alert twice")
	}
	if !m.rememberLoginIP(ctx, "alice@example.com", "5.6.7.8") {
		t.Fatal("a second new IP must alert")
	}
	var user models.User
	if err := db.First(&user, "email = ?", "alice@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if user.KnownIPs != "1.2.3.4,5.6.7.8" {
		t.Fatalf("known ips = %q", user.KnownIPs)
	}
}
