package mail

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// Send delivers a message via the submission port using the temp token auth.
// to/cc/bcc are the recipients: bcc appears in the SMTP envelope only, never
// in the message headers. When html is non-empty the body is sent as
// multipart/alternative; when attachments are present the whole message
// becomes multipart/mixed so both plain-text and rich-text stay intact.
// from is the envelope/From sender; email is the authenticated account.
func (c *Client) Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []Attachment) error {
	host := c.SMTPAddr
	serverHost := host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		serverHost = host[:i]
	}

	conn, err := smtp.Dial(host)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()

	tlsErr := conn.StartTLS(&tls.Config{InsecureSkipVerify: true, ServerName: serverHost})
	var auth smtp.Auth
	if tlsErr == nil {
		auth = smtp.PlainAuth("", email, token, serverHost)
	} else {
		// MAILEZ_TLS=off deployments accept plaintext on the internal
		// submission port; net/smtp refuses PlainAuth over plaintext, so use
		// the explicit AUTH PLAIN form.
		auth = NewPlainAuth(email, token)
	}
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := conn.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)
	for _, rcpt := range recipients {
		rcpt = strings.TrimSpace(rcpt)
		if rcpt == "" {
			continue
		}
		if err := conn.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", rcpt, err)
		}
	}
	wc, err := conn.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	msg := BuildMessage(from, to, cc, subject, text, html, attachments)
	if _, err := wc.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return conn.Quit()
}

// headerValue keeps the first line of a user-controlled header field so CR/LF
// cannot inject extra headers (SMTP header splitting). Anything after the
// first newline — including a fake "injected body" — is dropped entirely.
func headerValue(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// BuildMessage renders an RFC 5322 message with Date and Message-ID headers.
// Attachments (base64 data on the wire) are embedded as multipart/mixed parts;
// the text/html body, when present, is a nested multipart/alternative part.
// Exported so the outbox (send-undo queue) can park the exact bytes that will
// later be submitted, without keeping compose state around.
func BuildMessage(from string, to, cc []string, subject, text, html string, attachments []Attachment) string {
	var b strings.Builder
	b.WriteString("From: " + headerValue(from) + "\r\n")
	if len(to) > 0 {
		b.WriteString("To: " + headerValue(strings.Join(to, ", ")) + "\r\n")
	}
	if len(cc) > 0 {
		b.WriteString("Cc: " + headerValue(strings.Join(cc, ", ")) + "\r\n")
	}
	b.WriteString("Subject: " + headerValue(subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + newMessageID(from) + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")

	if len(attachments) == 0 {
		if html != "" {
			boundary := newBoundary()
			b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
			b.WriteString("\r\n")
			writePart(&b, boundary, "text/plain", text)
			writePart(&b, boundary, "text/html", html)
			b.WriteString("--" + boundary + "--\r\n")
		} else {
			b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
			b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
			b.WriteString("\r\n")
			b.WriteString(text)
			b.WriteString("\r\n")
		}
		return b.String()
	}

	outer := newBoundary()
	inner := newBoundary()
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + outer + "\"\r\n")
	b.WriteString("\r\n")
	if html != "" {
		b.WriteString("--" + outer + "\r\n")
		b.WriteString("Content-Type: multipart/alternative; boundary=\"" + inner + "\"\r\n")
		b.WriteString("\r\n")
		writePart(&b, inner, "text/plain", text)
		writePart(&b, inner, "text/html", html)
		b.WriteString("--" + inner + "--\r\n")
	} else {
		writePart(&b, outer, "text/plain", text)
	}
	for _, a := range attachments {
		writeAttachmentPart(&b, outer, a)
	}
	b.WriteString("--" + outer + "--\r\n")
	return b.String()
}

func writePart(b *strings.Builder, boundary, contentType, body string) {
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: " + contentType + "; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
}

func writeAttachmentPart(b *strings.Builder, boundary string, a Attachment) {
	ct := headerValue(a.ContentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	filename := a.Filename
	if filename == "" {
		filename = "attachment"
	}
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: " + ct + "; " + mimeFilename(filename) + "\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Content-Disposition: attachment; " + mimeFilename(filename) + "\r\n")
	b.WriteString("\r\n")
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(a.Data))
	if err != nil {
		raw = []byte(a.Data)
	}
	writeBase64(b, raw)
	b.WriteString("\r\n")
}

func writeBase64(b *strings.Builder, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
}

// mimeFilename renders a filename= parameter, switching to RFC 2231
// filename*=UTF-8” encoding for non-ASCII names (Chinese filenames etc.).
func mimeFilename(fn string) string {
	ascii := true
	for _, r := range fn {
		if r < 0x20 || r > 0x7e {
			ascii = false
			break
		}
	}
	if ascii && !strings.ContainsAny(fn, "\"\\") {
		return "filename=\"" + fn + "\""
	}
	return "filename*=UTF-8''" + url.QueryEscape(fn)
}

// newMessageID builds a random Message-ID using the sender's domain.
func newMessageID(from string) string {
	domain := "mailez"
	if i := strings.LastIndex(from, "@"); i >= 0 {
		if d := strings.TrimSpace(strings.Trim(from[i+1:], " >")); d != "" {
			domain = sanitizeDomain(d)
		}
	}
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return hex.EncodeToString([]byte("mailez")) + "@" + domain
	}
	return hex.EncodeToString(id) + "@" + domain
}

func sanitizeDomain(d string) string {
	var b strings.Builder
	for _, r := range d {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "mailez"
	}
	return b.String()
}

func newBoundary() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "mailez"
	}
	return hex.EncodeToString(b)
}
