package compose

import (
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
	user := currentUser(c)
	token, err := h.mailToken(c)
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
	uid, err := h.Mail.SaveDraft(user.Email, token, in.To, in.Cc, in.Subject, in.Text, in.HTML, in.Attachments, in.ReplaceUID)
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
	token, err := h.mailToken(c)
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
	}
	if err := c.BodyParser(&in); err != nil || len(in.To) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "to is required"})
	}
	from := in.From
	if from == "" {
		from = user.Email
	}
	if !alias.MaySendAs(h.App, user, from) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot send as this identity"})
	}
	// An undo window parks the message in the outbox instead of submitting it
	// right away; a worker delivers it once the window elapses.
	if in.UndoSeconds > 0 {
		if in.UndoSeconds > maxUndoSeconds {
			in.UndoSeconds = maxUndoSeconds
		}
		id, err := h.enqueue(user.Email, from, in.To, in.Cc, in.Bcc, in.Subject, in.Body, in.HTML, in.Attachments, in.UndoSeconds)
		if err != nil {
			return core.Fail(c, 500, err, "outbox error")
		}
		return c.JSON(fiber.Map{"queued": true, "outbox_id": id, "undo_seconds": in.UndoSeconds})
	}
	if err := h.Mail.Send(user.Email, token, from, in.To, in.Cc, in.Bcc, in.Subject, in.Body, in.HTML, in.Attachments); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
