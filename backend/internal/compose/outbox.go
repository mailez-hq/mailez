package compose

import (
	"context"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

// maxUndoSeconds bounds the send-undo window the client may request.
const maxUndoSeconds = 30

// OutboxWorker delivers due outbox entries through the local MTA. Submitting
// straight to the MTA port (like the fetch poller) needs no per-session SMTP
// token, so parked messages survive page reloads and backend restarts.
type OutboxWorker struct {
	DB      *gorm.DB
	MtaAddr string
}

func NewOutboxWorker(db *gorm.DB, mtaAddr string) *OutboxWorker {
	return &OutboxWorker{DB: db, MtaAddr: mtaAddr}
}

// Run polls for due messages until the context is cancelled.
func (w *OutboxWorker) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
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

// flush delivers every pending entry whose undo window has elapsed.
func (w *OutboxWorker) flush() {
	var due []models.Outbox
	if err := w.DB.Where("status = ? AND send_after <= ?", "pending", time.Now()).
		Order("send_after").Limit(10).Find(&due).Error; err != nil {
		return
	}
	for _, o := range due {
		if err := deliverOutbox(w.MtaAddr, o.FromAddr, strings.Split(o.Recipients, ","), o.RawMessage); err != nil {
			log.Printf("outbox %d delivery failed: %v", o.ID, err)
			w.DB.Model(&models.Outbox{}).Where("id = ?", o.ID).
				Updates(map[string]any{"status": "failed", "error": err.Error()})
			continue
		}
		w.DB.Model(&models.Outbox{}).Where("id = ?", o.ID).
			Updates(map[string]any{"status": "sent", "error": ""})
	}
}

// deliverOutbox submits a parked message to the local MTA; trailing CRLF is
// ensured per RFC 5321.
func deliverOutbox(addr, from string, recipients []string, raw string) error {
	msg := []byte(raw)
	if !strings.HasSuffix(raw, "\r\n") {
		msg = append(msg, '\r', '\n')
	}
	return smtp.SendMail(addr, nil, from, recipients, msg)
}

// outboxCancel lets the sender undo a parked message within the window.
// @Summary Undo a queued send
// @Tags mail
// @Success 204
// @Failure 404 {object} map[string]interface{}
// @Router /mail/outbox/{id} [delete]
func (h *Handler) outboxCancel(c *fiber.Ctx) error {
	user := currentUser(c)
	res := h.DB.Model(&models.Outbox{}).
		Where("id = ? AND account_email = ? AND status = ?", c.Params("id"), user.Email, "pending").
		Update("status", "cancelled")
	if res.Error != nil || res.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "nothing to undo"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// enqueue parks a fully built message for undoSeconds and returns its id.
func (h *Handler) enqueue(userEmail, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, undoSeconds int) (uint, error) {
	raw := mail.BuildMessage(from, to, cc, subject, text, html, attachments)
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)
	entry := models.Outbox{
		AccountEmail: userEmail,
		FromAddr:     from,
		Recipients:   strings.Join(recipients, ","),
		RawMessage:   raw,
		SendAfter:    time.Now().Add(time.Duration(undoSeconds) * time.Second),
		Status:       "pending",
	}
	if err := h.DB.Create(&entry).Error; err != nil {
		return 0, err
	}
	return entry.ID, nil
}
