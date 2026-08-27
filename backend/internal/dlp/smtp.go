package dlp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/smtp"
	"strings"
	"time"

	"mailez/backend/internal/core/models"
)

func jsonUnmarshal(b []byte, out any) error {
	return json.Unmarshal(b, out)
}

func readLimited(r io.Reader, n int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, n))
}

func logNotify(kind, approver string, id uint, err error) {
	log.Printf("dlp %s to %s for pending %d failed: %v", kind, approver, id, err)
}

// sendRaw delivers a raw RFC 5322 message to the local MTA (same contract as
// the outbox worker: internal link, opportunistic STARTTLS).
func (s *Service) sendRaw(from string, to []string, raw []byte) error {
	if s.send != nil {
		return s.send(from, to, raw)
	}
	msg := raw
	if !strings.HasSuffix(string(raw), "\r\n") {
		msg = append(msg, '\r', '\n')
	}
	c, err := smtp.Dial(s.Cfg.MailMtaAddr)
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
	for _, rcpt := range to {
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
	return w.Close()
}

// sendApprovalNotice emails one approver about a pending message.
func (s *Service) sendApprovalNotice(ctx context.Context, approver string, p *models.PendingApproval, rule models.DlpRule) error {
	from := "postmaster@" + s.Cfg.Domain
	subject := fmt.Sprintf("[审批] 邮件待审批：%s", p.Subject)
	body := fmt.Sprintf(
		"发件人：%s\n收件人：%s\n主题：%s\n\n命中规则：%s（%s）\n原因：%s\n\n请在 %s 前登录管理台审批。\n",
		p.From, p.Recipients, p.Subject, rule.Name, rule.Action, rule.Note, p.ExpiresAt.Format(time.RFC3339),
	)
	raw := buildNotice(from, approver, subject, body)
	return s.sendRaw(from, []string{approver}, raw)
}

// notifySender emails the original sender about a reject/expiry.
func (s *Service) notifySender(p *models.PendingApproval, kind string) {
	from := "postmaster@" + s.Cfg.Domain
	var subject, body string
	if kind == "expired" {
		subject = "[审批] 邮件已超时退回"
		body = fmt.Sprintf("您的邮件（主题：%s，发件人：%s）因审批超时未获批准，已被退回。\n", p.Subject, p.From)
	} else {
		subject = "[审批] 邮件审批未通过"
		body = fmt.Sprintf("您的邮件（主题：%s，发件人：%s）未通过审批。\n审批人：%s\n原因：%s\n", p.Subject, p.From, p.Approver, p.Reason)
	}
	raw := buildNotice(from, p.SenderEmail, subject, body)
	if err := s.sendRaw(from, []string{p.SenderEmail}, raw); err != nil {
		logNotify("reject notice", p.SenderEmail, p.ID, err)
	}
}

func buildNotice(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&b, "\r\n%s\r\n", body)
	return []byte(b.String())
}

// markOutboxRejected updates the originating outbox entry when the approval
// is rejected, so the sender's UI shows the final state.
func (s *Service) markOutboxRejected(pendingID uint, reason string) {
	s.DB.Model(&models.Outbox{}).
		Where("status = ? AND error = ?", "held", fmt.Sprintf("dlp_hold:%d", pendingID)).
		Updates(map[string]any{"status": "failed", "error": "rejected:" + reason})
}

// markOutboxApproved marks the originating outbox entry sent after delivery.
func (s *Service) markOutboxApproved(pendingID uint) {
	s.DB.Model(&models.Outbox{}).
		Where("status = ? AND error = ?", "held", fmt.Sprintf("dlp_hold:%d", pendingID)).
		Updates(map[string]any{"status": "sent", "error": ""})
}
