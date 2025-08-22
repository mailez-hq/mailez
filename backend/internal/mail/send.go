package mail

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"strings"
)

// Send delivers a message via the submission port using the temp token auth.
// When html is non-empty the body is sent as multipart/alternative so clients
// receive both the plain-text and the rich-text representation.
func (c *Client) Send(email, token, to, subject, text, html string) error {
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

	if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true, ServerName: serverHost}); err != nil {
		return fmt.Errorf("smtp starttls: %w", err)
	}
	auth := smtp.PlainAuth("", email, token, serverHost)
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := conn.Mail(email); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := conn.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	wc, err := conn.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	msg := buildMessage(email, to, subject, text, html)
	if _, err := wc.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return conn.Quit()
}

func buildMessage(from, to, subject, text, html string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
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

func writePart(b *strings.Builder, boundary, contentType, body string) {
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: " + contentType + "; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
}

func newBoundary() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "mailess"
	}
	return hex.EncodeToString(b)
}
