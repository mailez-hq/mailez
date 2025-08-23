package compose

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/mail"
)

// maxUndoSeconds bounds the send-undo window the client may request.
const maxUndoSeconds = 30

// OutboxWorker delivers due outbox entries through the local MTA. Submitting
// straight to the MTA port (like the fetch poller) needs no per-session SMTP
// token, so parked messages survive page reloads and backend restarts. Entries
// from external (aggregated) accounts are submitted to that account's SMTP
// server instead, using the decrypted stored credentials.
type OutboxWorker struct {
	DB        *gorm.DB
	MtaAddr   string
	SecretKey string
}

func NewOutboxWorker(db *gorm.DB, mtaAddr, secretKey string) *OutboxWorker {
	return &OutboxWorker{DB: db, MtaAddr: mtaAddr, SecretKey: secretKey}
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
		recipients := strings.Split(o.Recipients, ",")
		var err error
		if o.AccountID != 0 {
			err = w.deliverExternal(o, recipients)
		} else {
			err = deliverOutbox(w.MtaAddr, o.FromAddr, recipients, o.RawMessage)
		}
		if err != nil {
			log.Printf("outbox %d delivery failed: %v", o.ID, err)
			w.DB.Model(&models.Outbox{}).Where("id = ?", o.ID).
				Updates(map[string]any{"status": "failed", "error": err.Error()})
			continue
		}
		w.DB.Model(&models.Outbox{}).Where("id = ?", o.ID).
			Updates(map[string]any{"status": "sent", "error": ""})
	}
}

// deliverExternal submits a parked message through an aggregated account's own
// SMTP server.
func (w *OutboxWorker) deliverExternal(o models.Outbox, recipients []string) error {
	var acc models.Account
	if err := w.DB.First(&acc, "id = ?", o.AccountID).Error; err != nil {
		return err
	}
	if !acc.Enabled {
		return fmt.Errorf("account %d is disabled", o.AccountID)
	}
	pw, err := crypto.Decrypt(w.SecretKey, acc.PasswordEnc)
	if err != nil {
		return err
	}
	d := mail.ExternalDial(acc.Email, acc.SmtpHost, acc.SmtpPort, acc.SmtpSecurity, acc.Username, pw)
	return mail.New("", "", "").SubmitRaw(d, o.FromAddr, recipients, o.RawMessage)
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

// outboxCancel lets the sender undo a parked message within the window or
// cancel a scheduled send before it fires.
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

// outboxList returns the current user's upcoming scheduled sends so the UI can
// show and cancel them.
// @Summary List scheduled sends
// @Tags mail
// @Success 200 {array} map[string]interface{}
// @Router /mail/scheduled [get]
func (h *Handler) outboxList(c *fiber.Ctx) error {
	user := currentUser(c)
	var entries []models.Outbox
	if err := h.DB.Where("account_email = ? AND status = ? AND send_after > ?", user.Email, "pending", time.Now()).
		Order("send_after").Find(&entries).Error; err != nil {
		return core.Fail(c, 500, err, "outbox query failed")
	}
	out := make([]fiber.Map, 0, len(entries))
	for _, e := range entries {
		out = append(out, fiber.Map{
			"id":         e.ID,
			"from":       e.FromAddr,
			"subject":    e.Subject,
			"send_at":    e.SendAfter,
			"recipients": strings.Split(e.Recipients, ","),
		})
	}
	return c.JSON(out)
}

// enqueue parks a fully built message for delivery at sendAt (now + undo
// window, or a future timestamp for scheduled sends) and returns its id.
// accountID selects an external aggregated account (0 = internal gateway).
func (h *Handler) enqueue(userEmail string, accountID uint, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, sendAt time.Time) (uint, error) {
	raw := mail.BuildMessage(from, to, cc, subject, text, html, attachments)
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)
	entry := models.Outbox{
		AccountEmail: userEmail,
		AccountID:    accountID,
		FromAddr:     from,
		Subject:      subject,
		Recipients:   strings.Join(recipients, ","),
		RawMessage:   raw,
		SendAfter:    sendAt,
		Status:       "pending",
	}
	if err := h.DB.Create(&entry).Error; err != nil {
		return 0, err
	}
	return entry.ID, nil
}
