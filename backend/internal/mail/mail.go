package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Message is the API-facing representation of a mail.
type Message struct {
	UID           uint32       `json:"uid"`
	ID            string       `json:"id"`
	Seq           uint32       `json:"seq"`
	Subject       string       `json:"subject"`
	From          []Address    `json:"from"`
	To            []Address    `json:"to"`
	Cc            []Address    `json:"cc,omitempty"`
	Date          time.Time    `json:"date"`
	Flags         []string     `json:"flags"`
	HasAttachment bool         `json:"has_attachment"`
	ThreadID      string       `json:"thread_id,omitempty"`
	ThreadCount   int          `json:"thread_count,omitempty"`
	ThreadLatest  bool         `json:"thread_latest,omitempty"`
	Folder        string       `json:"folder,omitempty"`
	TextBody      string       `json:"text_body,omitempty"`
	HTMLBody      string       `json:"html_body,omitempty"`
	Attachments   []Attachment `json:"attachments,omitempty"`
}

// Attachment is one file embedded in a message, base64-encoded for download.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	Data        string `json:"data,omitempty"`
}

// Address is a mail address with an optional display name.
type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Client is a stateless IMAP gateway. Each operation opens its own connection
// authenticated with a per-session temp token (never the user's password).
type Client struct {
	IMAPAddr  string // host:port, e.g. gateway:10143
	SMTPAddr  string // host:port, e.g. gateway:10025
	SieveAddr string // host:port, e.g. gateway:4190

	// msgIDToUID caches the most recent (folder, Message-ID → UID) resolutions so
	// the reverse lookup served by UIDByMessageID is O(1) for hot messages
	// instead of scanning the whole mailbox every time.
	msgIDToUID msgIDCache
}

// New creates a mail gateway client.
func New(imapAddr, smtpAddr, sieveAddr string) *Client {
	return &Client{IMAPAddr: imapAddr, SMTPAddr: smtpAddr, SieveAddr: sieveAddr}
}

func (c *Client) tlsConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // internal connections only
}

// openIMAP dials the gateway IMAP proxy, upgrades to STARTTLS and logs in.
func (c *Client) openIMAP(email, token string) (*client.Client, error) {
	cli, err := client.Dial(c.IMAPAddr)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	if err := cli.StartTLS(c.tlsConfig()); err != nil {
		// TLS_FLAVOR=notls deployments serve plaintext on the internal proxy
		// port; fall back to the trusted internal link without encryption.
		// Log the miss so a misconfigured gateway is visible in operations.
		log.Printf("imap %s: STARTTLS unavailable, continuing in plaintext: %v", c.IMAPAddr, err)
	}
	if err := cli.Login(email, token); err != nil {
		_ = cli.Logout()
		return nil, fmt.Errorf("imap login: %w", err)
	}
	return cli, nil
}

// ListFolders returns all mailbox names.
func (c *Client) ListFolders(email, token string) ([]string, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() { done <- cli.List("", "*", mailboxes) }()

	var folders []string
	// Canonicalize the protocol-reserved INBOX to the display-friendly
	// "Inbox" so the mailbox list, URL paths and UI share one spelling that
	// matches the other folders' Title-case names. Every IMAP call later
	// normalizes back via inboxName().
	for m := range mailboxes {
		name := m.Name
		if strings.EqualFold(name, "inbox") {
			name = "Inbox"
		}
		folders = append(folders, name)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap list: %w", err)
	}
	return folders, nil
}

// PageSize bounds the number of messages returned per folder view.
const PageSize = 50

// ListMessages returns a page of the most recent messages of a folder (envelope
// only). Page is zero-based; the total message count is returned alongside.
func (c *Client) ListMessages(email, token, folder string, page int) ([]Message, int, error) {
	folder = inboxName(folder)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, 0, err
	}
	defer cli.Logout()

	mbox, err := cli.Select(folder, true)
	if err != nil {
		return nil, 0, fmt.Errorf("imap select %q: %w", folder, err)
	}
	if mbox.Messages == 0 || page < 0 {
		return []Message{}, int(mbox.Messages), nil
	}

	end := mbox.Messages - uint32(page)*PageSize
	if end == 0 {
		return []Message{}, int(mbox.Messages), nil
	}
	start := uint32(1)
	if end > PageSize {
		start = end - PageSize + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, end)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure}, messages)
	}()

	var out []Message
	for msg := range messages {
		out = append(out, envelopeToMessage(msg))
	}
	if err := <-done; err != nil {
		return nil, 0, fmt.Errorf("imap fetch: %w", err)
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	// Annotate the page with conversation metadata gathered from a wider
	// window, so the UI can show "N in thread" and walk the conversation.
	if len(out) > 0 {
		if meta, err := c.threadMeta(cli, mbox.Messages); err == nil {
			for i := range out {
				if tid, ok := meta.ids[out[i].UID]; ok {
					out[i].ThreadID = tid
					out[i].ThreadCount = meta.counts[tid]
					out[i].ThreadLatest = meta.latest[tid] == out[i].UID
				}
			}
		}
	}
	return out, int(mbox.Messages), nil
}

// GetMessage returns a full message body by UID.
func (c *Client) GetMessage(email, token, folder string, uid uint32) (*Message, error) {
	folder = inboxName(folder)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, true); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, section.FetchItem()}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() { done <- cli.UidFetch(seqset, items, messages) }()

	var msg *imap.Message
	for m := range messages {
		msg = m
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap uidfetch: %w", err)
	}
	if msg == nil {
		return nil, errors.New("message not found")
	}

	out := envelopeToMessage(msg)
	out.ThreadID = threadID(out.Subject)
	if body := msg.GetBody(section); body != nil {
		if textBody, htmlBody, attachments, err := extractBody(body); err == nil {
			out.TextBody = textBody
			out.HTMLBody = htmlBody
			out.Attachments = attachments
		}
	}
	return &out, nil
}

// GetRaw returns the full RFC 822 source of a message, for the "view raw"
// feature. Fetching the whole body works through the same BodySectionName the
// detail view uses.
func (c *Client) GetRaw(email, token, folder string, uid uint32) (string, error) {
	folder = inboxName(folder)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return "", err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, true); err != nil {
		return "", fmt.Errorf("imap select %q: %w", folder, err)
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seqset, []imap.FetchItem{section.FetchItem()}, messages)
	}()

	var raw string
	for msg := range messages {
		if body := msg.GetBody(section); body != nil {
			if b, err := io.ReadAll(body); err == nil {
				raw = string(b)
			}
		}
	}
	if err := <-done; err != nil {
		return "", fmt.Errorf("imap uidfetch raw: %w", err)
	}
	if raw == "" {
		return "", errors.New("message not found")
	}
	return raw, nil
}

func envelopeToMessage(msg *imap.Message) Message {
	out := Message{
		UID:   msg.Uid,
		Seq:   msg.SeqNum,
		Flags: msg.Flags,
	}
	if msg.Envelope != nil {
		out.Subject = msg.Envelope.Subject
		out.Date = msg.Envelope.Date
		out.From = addresses(msg.Envelope.From)
		out.To = addresses(msg.Envelope.To)
		out.Cc = addresses(msg.Envelope.Cc)
		out.ID = EncodeMessageID(msg.Envelope.MessageId)
	}
	// A mail without a Message-ID header has no stable key to derive the
	// routable id from. Fall back to the numeric UID so the row still gets a
	// valid (non-degenerate) id that the frontend can route by; the uid is
	// guessed directly by the uid fetch path and needs no reverse lookup.
	if out.ID == "." {
		out.ID = strconv.FormatUint(uint64(msg.Uid), 10)
	}
	if msg.BodyStructure != nil {
		out.HasAttachment = hasAttachments(msg.BodyStructure)
	}
	return out
}

func addresses(list []*imap.Address) []Address {
	var out []Address
	for _, a := range list {
		out = append(out, Address{Name: a.PersonalName, Email: a.MailboxName + "@" + a.HostName})
	}
	return out
}

func hasAttachments(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	if bs.MIMEType == "multipart" {
		for _, part := range bs.Parts {
			if part.Disposition == "attachment" || (part.MIMEType == "multipart" && hasAttachments(part)) {
				return true
			}
		}
		return false
	}
	return bs.Disposition == "attachment"
}

// extractBody walks a MIME body and returns the plain-text part, the HTML part
// and any attachments (decoded and base64-encoded for transport).
func extractBody(r io.Reader) (text, html string, attachments []Attachment, err error) {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return "", "", nil, err
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		mt = "text/plain"
	}
	body := msg.Body
	encoding := msg.Header.Get("Content-Transfer-Encoding")

	if strings.HasPrefix(mt, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			disposition, dparams, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			filename := dparams["filename"]
			b, _ := io.ReadAll(part)
			decoded := []byte(decodeBody(b, part.Header.Get("Content-Transfer-Encoding")))
			if disposition == "attachment" || filename != "" {
				attachments = append(attachments, Attachment{
					Filename:    filename,
					ContentType: partType,
					Size:        len(decoded),
					Data:        base64.StdEncoding.EncodeToString(decoded),
				})
			} else if strings.HasPrefix(partType, "text/plain") && text == "" {
				text = string(decoded)
			} else if strings.HasPrefix(partType, "text/html") && html == "" {
				html = SanitizeHTML(string(decoded))
			}
		}
	} else {
		b, _ := io.ReadAll(body)
		if strings.HasPrefix(mt, "text/plain") {
			text = decodeBody(b, encoding)
		} else if strings.HasPrefix(mt, "text/html") {
			html = SanitizeHTML(decodeBody(b, encoding))
		}
	}
	return text, html, attachments, nil
}

func decodeBody(b []byte, encoding string) string {
	switch strings.ToLower(encoding) {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err == nil {
			return string(decoded)
		}
	case "quoted-printable":
		r := quotedprintable.NewReader(bytes.NewReader(b))
		decoded, err := io.ReadAll(r)
		if err == nil {
			return string(decoded)
		}
	}
	return string(b)
}
