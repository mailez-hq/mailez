package mail

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Send delivers a message via the submission port using the temp token auth.
// to/cc/bcc are the recipients: bcc appears in the SMTP envelope only, never
// in the message headers. When html is non-empty the body is sent as
// multipart/alternative; when attachments are present the whole message
// becomes multipart/mixed so both plain-text and rich-text stay intact.
// from is the envelope/From sender; email is the authenticated account.
// Header is an extra RFC 5322 header pair appended to a built message
// (e.g. In-Reply-To / References for reply threading).
type Header struct {
	Key   string
	Value string
}

func (c *Client) Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []Attachment, extra ...Header) error {
	conn, err := c.openSMTP(email, token)
	if err != nil {
		return err
	}
	defer conn.Close()
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)
	raw := BuildMessage(from, to, cc, subject, text, html, attachments, extra...)
	if err := submitSMTP(conn, from, recipients, raw); err != nil {
		return err
	}
	// Keep a copy in Sent Items. The engine does not auto-copy submissions,
	// so without this the webmail Sent folder would stay empty (and recall
	// would have nothing to flag). Best-effort: a failed copy must not fail
	// the send itself.
	if !c.dial.External {
		if err := c.AppendRaw(email, token, "Sent", raw, nil); err != nil {
			log.Printf("mail: save sent copy for %s: %v", email, err)
		}
	}
	return nil
}

// SubmitRawAs delivers a pre-built RFC 5322 message through the gateway's
// submission server authenticated as the given account. ActiveSync clients
// (SendMail/SmartReply/SmartForward) supply the full MIME, so the backend
// must submit it verbatim instead of rebuilding headers/body.
func (c *Client) SubmitRawAs(email, token, from string, recipients []string, raw string) error {
	conn, err := c.openSMTP(email, token)
	if err != nil {
		return err
	}
	defer conn.Close()
	if from == "" {
		if msg, perr := netmail.ReadMessage(strings.NewReader(raw)); perr == nil {
			from = msg.Header.Get("From")
		}
	}
	if len(recipients) == 0 {
		if msg, perr := netmail.ReadMessage(strings.NewReader(raw)); perr == nil {
			recipients = envelopeRecipients(msg.Header)
		}
	}
	return submitSMTP(conn, from, recipients, raw)
}

// AppendRaw stores a pre-built RFC 5322 message into a folder with IMAP
// APPEND. The ActiveSync SendMail path uses it to honour SaveInSentItems.
func (c *Client) AppendRaw(email, token, folder, raw string, flags []string) error {
	folder = inboxName(folder)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if len(flags) == 0 {
		flags = []string{`\Seen`}
	}
	if err := cli.Append(folder, flags, time.Now(), strings.NewReader(raw)); err != nil {
		return fmt.Errorf("imap append %q: %w", folder, err)
	}
	return nil
}

// envelopeRecipients collects To/Cc/Bcc from parsed message headers.
func envelopeRecipients(h netmail.Header) []string {
	var out []string
	for _, field := range []string{"To", "Cc", "Bcc"} {
		addrs, _ := h.AddressList(field)
		for _, addr := range addrs {
			if addr.Address != "" {
				out = append(out, addr.Address)
			}
		}
	}
	return out
}

// SubmitRaw delivers a pre-built RFC 5322 message through the dial's
// submission server without rebuilding it; the outbox worker uses it to deliver
// parked messages from external (aggregated) accounts.
func (c *Client) SubmitRaw(d Dial, from string, recipients []string, raw string) error {
	conn, err := openExternalSMTP(d)
	if err != nil {
		return err
	}
	defer conn.Close()
	return submitSMTP(conn, from, recipients, raw)
}

// submitSMTP runs the envelope/data phase of an SMTP transaction for an already
// connected and authenticated client.
func submitSMTP(conn *smtp.Client, from string, recipients []string, msg string) error {
	if err := conn.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
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
	if _, err := wc.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return conn.Quit()
}

// openSMTP connects and authenticates to the submission server. External
// (aggregated) accounts use their own host/security/credentials; internal
// accounts use the gateway with the per-session temp token.
func (c *Client) openSMTP(email, token string) (*smtp.Client, error) {
	if c.dial.External {
		return openExternalSMTP(c.dial)
	}
	host := c.SMTPAddr
	serverHost := host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		serverHost = host[:i]
	}

	conn, err := smtp.Dial(host)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
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
		conn.Close()
		return nil, fmt.Errorf("smtp auth: %w", err)
	}
	return conn, nil
}

// openExternalSMTP connects to an external submission server honouring its
// security policy and authenticates with the stored credentials.
func openExternalSMTP(d Dial) (*smtp.Client, error) {
	addr := net.JoinHostPort(d.Host, strconv.Itoa(d.Port))
	tlsCfg := &tls.Config{InsecureSkipVerify: true, ServerName: d.Host}
	var raw net.Conn
	var err error
	switch d.Security {
	case "tls", "ssl", "smtps":
		raw, err = tls.Dial("tcp", addr, tlsCfg)
	default: // none, starttls: start plain, upgrade below
		raw, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	cli, err := smtp.NewClient(raw, d.Host)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	if d.Security == "starttls" {
		if err := cli.StartTLS(tlsCfg); err != nil {
			cli.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if err := cli.Auth(smtp.PlainAuth("", d.Username, d.Password, d.Host)); err != nil {
		cli.Close()
		return nil, fmt.Errorf("smtp auth: %w", err)
	}
	return cli, nil
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
func BuildMessage(from string, to, cc []string, subject, text, html string, attachments []Attachment, extra ...Header) string {
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
	for _, h := range extra {
		if h.Key != "" && h.Value != "" {
			b.WriteString(h.Key + ": " + headerValue(h.Value) + "\r\n")
		}
	}
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
