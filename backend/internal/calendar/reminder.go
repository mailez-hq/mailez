package calendar

// Event reminder worker: when an event's start minus its reminder window
// arrives, the owner gets a mail delivered through the local MTA (which also
// triggers the existing new-mail web-push path).
import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

// ReminderWorker scans for due reminders and delivers them once per event.
type ReminderWorker struct {
	DB  *gorm.DB
	Cfg core.Config
}

// NewReminderWorker wires the worker.
func NewReminderWorker(db *gorm.DB, cfg core.Config) *ReminderWorker {
	return &ReminderWorker{DB: db, Cfg: cfg}
}

// Run polls for due reminders until the context is cancelled.
func (w *ReminderWorker) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.flush()
		}
	}
}

func (w *ReminderWorker) flush() {
	// Singleton via DB lease: the reminder-log check is check-then-insert,
	// so two replicas flushing the same event could deliver twice. The
	// lease keeps one flusher; the log keeps the once-per-event guarantee.
	if !cluster.TryHold(w.DB, "calendar_reminder", cluster.LeaseTTL) {
		return
	}
	now := time.Now()
	var events []models.CalendarEvent
	if err := w.DB.Where("reminder_minutes > 0 AND start IS NOT NULL").Find(&events).Error; err != nil {
		return
	}
	for _, ev := range events {
		fireAt := ev.Start.Add(-time.Duration(ev.ReminderMinutes) * time.Minute)
		if fireAt.After(now) {
			continue
		}
		var sent models.CalendarReminderLog
		err := w.DB.Where("event_id = ?", ev.ID).First(&sent).Error
		if err == nil {
			continue // already delivered
		}
		if err := w.deliver(ev); err != nil {
			log.Printf("calendar reminder %d (%s): %v", ev.ID, ev.Summary, err)
			continue
		}
		entry := models.CalendarReminderLog{
			EventID:   ev.ID,
			UserEmail: ev.UserEmail,
			FireAt:    fireAt,
			SentAt:    &now,
		}
		if err := w.DB.Create(&entry).Error; err != nil {
			log.Printf("calendar reminder log %d: %v", ev.ID, err)
		}
	}
}

func (w *ReminderWorker) deliver(ev models.CalendarEvent) error {
	subject := "日程提醒：" + ev.Summary
	text := fmt.Sprintf("您有一个日程即将开始：\n\n%s\n", ev.Summary)
	if ev.Start != nil {
		text += "时间：" + ev.Start.Local().Format("2006-01-02 15:04")
		if ev.End != nil {
			text += " - " + ev.End.Local().Format("2006-01-02 15:04")
		}
		text += "\n"
	}
	if ev.Location != "" {
		text += "地点：" + ev.Location + "\n"
	}
	if ev.Description != "" {
		text += "\n" + ev.Description + "\n"
	}
	raw := mail.BuildMessage(ev.UserEmail, []string{ev.UserEmail}, nil, subject, text, "", nil)
	return submitMTA(w.Cfg.MailMtaAddr, ev.UserEmail, []string{ev.UserEmail}, raw)
}

// submitMTA delivers a pre-built message through the local MTA (trusted
// internal link, no authentication), mirroring the outbox worker.
var submitMTA = realSubmitMTA

func realSubmitMTA(addr, from string, recipients []string, raw string) error {
	msg := []byte(raw)
	if !strings.HasSuffix(raw, "\r\n") {
		msg = append(msg, '\r', '\n')
	}
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
		// plaintext internal link is fine
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range recipients {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
