package compose

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/alias"
	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"
)

// mailSaveDraft stores a working draft in Drafts; replace_uid (the UID of the
// previous auto-save) replaces it so each compose session keeps one draft.
// mailSaveDraft stores a working draft in Drafts.
// @Summary Save draft
// @Tags mail
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "uid"
// @Failure 400 {object} map[string]interface{}
// @Router /mail/draft [post]
func (h *Handler) mailSaveDraft(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Subject     string            `json:"subject"`
		To          core.StringList   `json:"to"`
		Cc          core.StringList   `json:"cc"`
		Text        string            `json:"text"`
		HTML        string            `json:"html"`
		ReplaceUID  uint32            `json:"replace_uid"`
		Attachments []mail.Attachment `json:"attachments"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	uid, err := h.Mail.With(d).SaveDraft(d.Email, d.Token, in.To, in.Cc, in.Subject, in.Text, in.HTML, in.Attachments, in.ReplaceUID)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"uid": uid})
}

// mailSend delivers a message on behalf of the current user, honouring the
// spoofing policy (own address or one of their aliases).
// mailSend delivers a message on behalf of the current user.
// @Summary Send message
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /mail/send [post]
func (h *Handler) mailSend(c *fiber.Ctx) error {
	user := currentUser(c)
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		From        string            `json:"from"`
		To          core.StringList   `json:"to"`
		Cc          core.StringList   `json:"cc"`
		Bcc         core.StringList   `json:"bcc"`
		Subject     string            `json:"subject"`
		Body        string            `json:"body"`
		HTML        string            `json:"html"`
		Attachments []mail.Attachment `json:"attachments"`
		UndoSeconds int               `json:"undo_seconds"`
		SendAt      string            `json:"send_at"` // RFC3339; future value schedules the send
		InReplyTo   string            `json:"in_reply_to"`
		References  string            `json:"references"`
		Receipt     bool              `json:"receipt_requested"`
	}
	if err := c.BodyParser(&in); err != nil || len(in.To) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "to is required"})
	}
	from := in.From
	if from == "" {
		from = d.Email
	}
	// Identity policy: an external aggregated account may only send as its own
	// address; the internal account may send as itself or one of its aliases.
	if d.External {
		if !strings.EqualFold(from, d.Email) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot send as this identity"})
		}
	} else if !alias.MaySendAs(h.App, user, from) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot send as this identity"})
	}
	// A future send_at parks the message in the outbox until that moment
	// (scheduled send); an undo window parks it for a few seconds instead.
	// The two are mutually exclusive: scheduling disables undo.
	if in.SendAt != "" {
		sendAt, err := time.Parse(time.RFC3339, in.SendAt)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid send_at"})
		}
		if !sendAt.After(time.Now()) {
			return c.Status(400).JSON(fiber.Map{"error": "send_at must be in the future"})
		}
		id, err := h.enqueue(user.Email, d.AccountID, from, in.To, in.Cc, in.Bcc, in.Subject, in.Body, in.HTML, in.Attachments, sendAt, in.InReplyTo, in.References, in.Receipt)
		if err != nil {
			return core.Fail(c, 500, err, "outbox error")
		}
		return c.JSON(fiber.Map{"scheduled": true, "outbox_id": id})
	}
	if in.UndoSeconds > 0 {
		if in.UndoSeconds > maxUndoSeconds {
			in.UndoSeconds = maxUndoSeconds
		}
		id, err := h.enqueue(user.Email, d.AccountID, from, in.To, in.Cc, in.Bcc, in.Subject, in.Body, in.HTML, in.Attachments, time.Now().Add(time.Duration(in.UndoSeconds)*time.Second), in.InReplyTo, in.References, in.Receipt)
		if err != nil {
			return core.Fail(c, 500, err, "outbox error")
		}
		return c.JSON(fiber.Map{"queued": true, "outbox_id": id, "undo_seconds": in.UndoSeconds})
	}
	extra := []mail.Header{}
	if in.InReplyTo != "" {
		extra = append(extra, mail.Header{Key: "In-Reply-To", Value: in.InReplyTo})
	}
	if in.References != "" {
		extra = append(extra, mail.Header{Key: "References", Value: in.References})
	}
	if in.Receipt {
		extra = append(extra, mail.Header{Key: "Disposition-Notification-To", Value: from})
	}
	if err := h.Mail.With(d).Send(d.Email, d.Token, from, in.To, in.Cc, in.Bcc, in.Subject, in.Body, in.HTML, in.Attachments, extra...); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
