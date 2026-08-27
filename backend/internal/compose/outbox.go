package compose

import (
	"context"
	"crypto/tls"
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
	"mailez/backend/internal/dlp"
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
	DLP       *dlp.Service // optional outbound content filter (审批/DLP)
}

func NewOutboxWorker(db *gorm.DB, mtaAddr, secretKey string, dlpSvc *dlp.Service) *OutboxWorker {
	return &OutboxWorker{DB: db, MtaAddr: mtaAddr, SecretKey: secretKey, DLP: dlpSvc}
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
	// Compare against UTC: SQLite compares these DATETIME columns as strings,
	// so the stored value and the query parameter must use the same zone
	// offset. enqueue() stores UTC; a local-zone time.Now() here would make
	// future scheduled sends compare as already-due and fire immediately.
	if err := w.DB.Where("status = ? AND send_after <= ?", "pending", time.Now().UTC()).
		Order("send_after").Limit(10).Find(&due).Error; err != nil {
		return
	}
	for _, o := range due {
		recipients := strings.Split(o.Recipients, ",")
		// Outbound content filter (敏感词/审批): internal messages are
		// scanned before submission; a block rejects, a hold parks the
		// message until an approver decides.
		if w.DLP != nil && o.AccountID == 0 {
			res, derr := w.DLP.CheckRaw(context.Background(), o.AccountEmail, o.FromAddr, recipients, []byte(o.RawMessage))
			if derr == nil && res != nil && res.Action != "pass" {
				if res.Action == "block" {
					w.DB.Model(&models.Outbox{}).Where("id = ?", o.ID).
						Updates(map[string]any{"status": "failed", "error": "blocked:" + res.Reason})
					continue
				}
				w.DB.Model(&models.Outbox{}).Where("id = ?", o.ID).
					Updates(map[string]any{"status": "held", "error": fmt.Sprintf("dlp_hold:%d", res.ID)})
				continue
			}
		}
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
	// The local MTA (postdove gateway or mailezine dev) may present a
	// self-signed certificate on the internal link; verify=false matches
	// the rest of the internal mail client (mail.go tlsConfig).
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	// Opportunistic STARTTLS with the internal (self-signed) cert tolerated.
	if err := c.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
		// plaintext internal link (postdove dev) is fine
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
	if err := h.DB.Where("account_email = ? AND status = ? AND send_after > ?", user.Email, "pending", time.Now().UTC()).
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
func (h *Handler) enqueue(userEmail string, accountID uint, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, sendAt time.Time, inReplyTo, references string) (uint, error) {
	extra := []mail.Header{}
	if inReplyTo != "" {
		extra = append(extra, mail.Header{Key: "In-Reply-To", Value: inReplyTo})
	}
	if references != "" {
		extra = append(extra, mail.Header{Key: "References", Value: references})
	}
	raw := mail.BuildMessage(from, to, cc, subject, text, html, attachments, extra...)
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
		// Normalize to UTC so the worker/list comparisons (which happen as
		// string comparisons in SQLite) are zone-consistent regardless of
		// whether the caller passed a local time (undo) or a parsed RFC3339
		// timestamp (scheduled send).
		SendAfter: sendAt.UTC(),
		Status:    "pending",
	}
	if err := h.DB.Create(&entry).Error; err != nil {
		return 0, err
	}
	return entry.ID, nil
}
