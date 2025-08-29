package compose

// Enterprise mail features: read receipts (RFC 3798), same-system message
// recall (X-MS-Recall notices) and mail merge (逐封群发).
import (
	"fmt"
	"log"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"
)

// registerEnterprise mounts the enterprise compose routes.
func (h *Handler) registerEnterprise(r fiber.Router) {
	r.Post("/mail/receipt", h.mailSendReceipt)
	r.Post("/mail/recall", h.mailRecall)
	r.Post("/mail/recall/apply", h.mailRecallApply)
	r.Post("/mail/merge", h.mailMerge)
}

// mailSendReceipt answers a read-receipt request: it mails a disposition
// notification (RFC 3798) to the original sender and marks the message with
// the $MDNSent keyword so the UI stops offering the button.
// @Summary Send read receipt
// @Tags mail
// @Accept json
// @Success 204
// @Router /mail/receipt [post]
func (h *Handler) mailSendReceipt(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	var in struct {
		Folder string `json:"folder"`
		UID    uint32 `json:"uid"`
		To     string `json:"to"` // the sender who asked for the receipt
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	raw, err := h.Mail.With(d).GetRaw(d.Email, d.Token, in.Folder, in.UID)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	msg, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "cannot parse message"})
	}
	messageID := strings.TrimSpace(msg.Header.Get("Message-ID"))
	subject := strings.TrimSpace(msg.Header.Get("Subject"))
	to := strings.TrimSpace(in.To)
	if to == "" {
		to = strings.TrimSpace(msg.Header.Get("Disposition-Notification-To"))
	}
	if to == "" {
		return c.Status(400).JSON(fiber.Map{"error": "receipt destination missing"})
	}
	text := fmt.Sprintf("您在 %s 发送的邮件「%s」已被收件人阅读。\n",
		nowString(), subject)
	extra := []mail.Header{
		{Key: "X-MDN-Disposition", Value: "manual-action/MDN-sent; displayed"},
	}
	if messageID != "" {
		extra = append(extra,
			mail.Header{Key: "In-Reply-To", Value: messageID},
			mail.Header{Key: "References", Value: messageID})
	}
	receiptSubject := "已读回执：" + subject
	if err := h.Mail.With(d).Send(d.Email, d.Token, d.Email, []string{to}, nil, nil,
		receiptSubject, text, "", nil, extra...); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if err := h.Mail.With(d).SetFlag(d.Email, d.Token, in.Folder, in.UID, `$MDNSent`, true); err != nil {
		log.Printf("compose: mark $MDNSent: %v", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailRecall recalls a sent message: it mails an X-MS-Recall notice to every
// recipient (To+Cc) and flags the sent copy so the UI shows "已撤回".
// @Summary Recall a sent message
// @Tags mail
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /mail/recall [post]
func (h *Handler) mailRecall(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	var in struct {
		Folder string `json:"folder"`
		UID    uint32 `json:"uid"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	raw, err := h.Mail.With(d).GetRaw(d.Email, d.Token, in.Folder, in.UID)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	msg, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "cannot parse message"})
	}
	messageID := strings.TrimSpace(msg.Header.Get("Message-ID"))
	subject := strings.TrimSpace(msg.Header.Get("Subject"))
	recipients := headerAddresses(msg.Header, "To")
	recipients = append(recipients, headerAddresses(msg.Header, "Cc")...)
	if len(recipients) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no recipients to recall"})
	}
	text := fmt.Sprintf("发件人申请撤回邮件「%s」，如您尚未阅读，可忽略或删除原邮件。\n", subject)
	extra := []mail.Header{
		{Key: "X-MS-Recall", Value: "yes"},
		{Key: "X-MS-Recall-MessageID", Value: messageID},
		{Key: "X-MS-Recall-Subject", Value: subject},
	}
	recallSubject := "撤回：" + subject
	if err := h.Mail.With(d).Send(d.Email, d.Token, d.Email, recipients, nil, nil,
		recallSubject, text, "", nil, extra...); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if err := h.Mail.With(d).SetFlag(d.Email, d.Token, in.Folder, in.UID, `$RecallSent`, true); err != nil {
		log.Printf("compose: mark $RecallSent: %v", err)
	}
	return c.JSON(fiber.Map{"notified": len(recipients)})
}

// mailRecallApply removes the original message that a recall notice targets.
// The reader calls it from the recall banner; the original is located by
// Message-ID across the visible folders.
// @Summary Apply a recall notice
// @Tags mail
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /mail/recall/apply [post]
func (h *Handler) mailRecallApply(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	var in struct {
		MessageID string `json:"message_id"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.MessageID) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "message_id is required"})
	}
	id := mail.EncodeMessageID(in.MessageID)
	folders, err := h.Mail.With(d).ListFolders(d.Email, d.Token)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	removed := 0
	for _, folder := range folders {
		if strings.EqualFold(folder, "Sent") || strings.EqualFold(folder, "Drafts") {
			continue
		}
		uid, err := h.Mail.With(d).UIDByMessageID(d.Email, d.Token, folder, id)
		if err != nil || uid == 0 {
			continue
		}
		if err := h.Mail.With(d).Delete(d.Email, d.Token, folder, uid); err == nil {
			removed++
		}
	}
	return c.JSON(fiber.Map{"removed": removed})
}

// mergeRecipient is one mail-merge target; vars feed {{name}}-style template
// placeholders (name/email are provided implicitly).
type mergeRecipient struct {
	Email string            `json:"email"`
	Name  string            `json:"name"`
	Vars  map[string]string `json:"vars"`
}

// mailMerge sends one personalized copy per recipient (逐封群发): the
// subject/body may contain {{name}}, {{email}} and any custom {{var}} from
// the recipient row. Each copy goes to a single To so recipients never see
// each other.
// @Summary Mail merge (逐封群发)
// @Tags mail
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /mail/merge [post]
func (h *Handler) mailMerge(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	var in struct {
		From        string            `json:"from"`
		Subject     string            `json:"subject"`
		Body        string            `json:"body"`
		HTML        string            `json:"html"`
		Attachments []mail.Attachment `json:"attachments"`
		Recipients  []mergeRecipient  `json:"recipients"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if len(in.Recipients) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "recipients are required"})
	}
	from := in.From
	if from == "" {
		from = d.Email
	}
	sent := 0
	var failed []fiber.Map
	for _, rcpt := range in.Recipients {
		email := strings.TrimSpace(rcpt.Email)
		if email == "" {
			continue
		}
		vars := map[string]string{
			"name":  rcpt.Name,
			"email": email,
		}
		for k, v := range rcpt.Vars {
			vars[strings.ToLower(strings.TrimSpace(k))] = v
		}
		subject := substitute(in.Subject, vars)
		text := substitute(in.Body, vars)
		html := substitute(in.HTML, vars)
		if err := h.Mail.With(d).Send(d.Email, d.Token, from, []string{email}, nil, nil,
			subject, text, html, in.Attachments); err != nil {
			log.Printf("compose: merge to %s: %v", email, err)
			failed = append(failed, fiber.Map{"email": email, "error": err.Error()})
			continue
		}
		sent++
	}
	return c.JSON(fiber.Map{"sent": sent, "failed": failed})
}

// substitute replaces {{key}} placeholders (case-insensitive) in a template.
func substitute(tpl string, vars map[string]string) string {
	for k, v := range vars {
		tpl = strings.ReplaceAll(tpl, "{{"+k+"}}", v)
		tpl = strings.ReplaceAll(tpl, "{{ "+k+" }}", v)
		tpl = strings.ReplaceAll(tpl, "{{"+toTitle(k)+"}}", v)
	}
	return tpl
}

func headerAddresses(h netmail.Header, field string) []string {
	addrs, _ := h.AddressList(field)
	var out []string
	for _, a := range addrs {
		if a.Address != "" {
			out = append(out, a.Address)
		}
	}
	return out
}

func nowString() string {
	return time.Now().Format("2006-01-02 15:04")
}

// toTitle uppercases the first letter (placeholder {{Name}} form).
func toTitle(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
