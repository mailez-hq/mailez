package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Message is the API-facing representation of a mail.
type Message struct {
	UID           uint32       `json:"uid"`
	Seq           uint32       `json:"seq"`
	Subject       string       `json:"subject"`
	From          []Address    `json:"from"`
	To            []Address    `json:"to"`
	Date          time.Time    `json:"date"`
	Flags         []string     `json:"flags"`
	HasAttachment bool         `json:"has_attachment"`
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
	IMAPAddr string // host:port, e.g. front:10143
	SMTPAddr string // host:port, e.g. front:10025
}

// New creates a mail gateway client.
func New(imapAddr, smtpAddr string) *Client {
	return &Client{IMAPAddr: imapAddr, SMTPAddr: smtpAddr}
}

func (c *Client) tlsConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // internal connections only
}

// openIMAP dials the front IMAP proxy, upgrades to STARTTLS and logs in.
func (c *Client) openIMAP(email, token string) (*client.Client, error) {
	cli, err := client.Dial(c.IMAPAddr)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	if err := cli.StartTLS(c.tlsConfig()); err != nil {
		_ = cli.Logout()
		return nil, fmt.Errorf("imap starttls: %w", err)
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
	for m := range mailboxes {
		folders = append(folders, m.Name)
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
	return out, int(mbox.Messages), nil
}

// SearchMessages returns messages matching a free-text query (subject, from,
// body) in a folder, newest first.
func (c *Client) SearchMessages(email, token, folder, query string) ([]Message, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, true); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}
	criteria := imap.NewSearchCriteria()
	criteria.Text = []string{query}
	uids, err := cli.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}
	if len(uids) == 0 {
		return []Message{}, nil
	}
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure}, messages)
	}()

	var out []Message
	for msg := range messages {
		out = append(out, envelopeToMessage(msg))
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap uidfetch: %w", err)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// GetMessage returns a full message body by UID.
func (c *Client) GetMessage(email, token, folder string, uid uint32) (*Message, error) {
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
	if body := msg.GetBody(section); body != nil {
		if textBody, htmlBody, attachments, err := extractBody(body); err == nil {
			out.TextBody = textBody
			out.HTMLBody = htmlBody
			out.Attachments = attachments
		}
	}
	return &out, nil
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
				html = string(decoded)
			}
		}
	} else {
		b, _ := io.ReadAll(body)
		if strings.HasPrefix(mt, "text/plain") {
			text = decodeBody(b, encoding)
		} else if strings.HasPrefix(mt, "text/html") {
			html = decodeBody(b, encoding)
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
