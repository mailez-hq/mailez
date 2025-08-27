package calendar

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func TestReminderWorkerFiresOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rem.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CalendarEvent{}, &models.CalendarReminderLog{}); err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	start := time.Now().Add(-time.Minute) // reminder is due
	if err := db.Create(&models.CalendarEvent{
		UserEmail: "alice@example.com", UID: "rem-1", Summary: "站会",
		Start: &start, ReminderMinutes: 5, ICS: "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
	}).Error; err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var delivered []string
	orig := submitMTA
	submitMTA = func(addr, from string, recipients []string, raw string) error {
		mu.Lock()
		delivered = append(delivered, from+"|"+strings.Join(recipients, ",")+"|"+raw)
		mu.Unlock()
		return nil
	}
	defer func() { submitMTA = orig }()

	w := NewReminderWorker(db, core.Config{MailMtaAddr: "mta:25"})
	w.flush()
	w.flush() // second pass must be a no-op thanks to the log row

	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != 1 {
		t.Fatalf("delivered = %d, want 1", len(delivered))
	}
	if !strings.Contains(delivered[0], "alice@example.com") || !strings.Contains(delivered[0], "站会") {
		t.Fatalf("reminder mail = %q", delivered[0])
	}
	var logCount int64
	db.Model(&models.CalendarReminderLog{}).Count(&logCount)
	if logCount != 1 {
		t.Fatalf("reminder log count = %d", logCount)
	}
}

func TestReminderNotDueYet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rem2.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CalendarEvent{}, &models.CalendarReminderLog{}); err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	start := time.Now().Add(time.Hour)
	if err := db.Create(&models.CalendarEvent{
		UserEmail: "alice@example.com", UID: "rem-2", Summary: "未来日程",
		Start: &start, ReminderMinutes: 10, ICS: "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
	}).Error; err != nil {
		t.Fatal(err)
	}
	orig := submitMTA
	submitMTA = func(addr, from string, recipients []string, raw string) error { return nil }
	defer func() { submitMTA = orig }()
	w := NewReminderWorker(db, core.Config{MailMtaAddr: "mta:25"})
	w.flush()
	var logCount int64
	db.Model(&models.CalendarReminderLog{}).Count(&logCount)
	if logCount != 0 {
		t.Fatalf("future reminder fired early: %d", logCount)
	}
}
