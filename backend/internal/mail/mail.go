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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	"mailez/backend/internal/mail/imaputf7"
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
	Bcc           []Address    `json:"bcc,omitempty"`
	Date          time.Time    `json:"date"`
	Flags         []string     `json:"flags"`
	HasAttachment bool         `json:"has_attachment"`
	Size          int64        `json:"size,omitempty"`
	ThreadID      string       `json:"thread_id,omitempty"`
	ThreadCount   int          `json:"thread_count,omitempty"`
	ThreadLatest  bool         `json:"thread_latest,omitempty"`
	// ThreadUnread / ThreadFlagged are conversation-level aggregates: any
	// member unread / starred (conversation view rows only).
	ThreadUnread  bool     `json:"thread_unread,omitempty"`
	ThreadFlagged bool     `json:"thread_flagged,omitempty"`
	// ThreadSenders lists the distinct senders of the conversation
	// (conversation view rows only).
	ThreadSenders []string `json:"thread_senders,omitempty"`
	Folder        string       `json:"folder,omitempty"`
	TextBody      string       `json:"text_body,omitempty"`
	HTMLBody      string       `json:"html_body,omitempty"`
	Attachments   []Attachment `json:"attachments,omitempty"`
	// List-Unsubscribe (RFC 2369) as exposed by the sender: an https URL the
	// backend can call on the user's behalf, or a mailto: the client turns
	// into a pre-filled compose. UnsubscribePost marks RFC 8058 one-click.
	UnsubscribeURL  string `json:"unsubscribe_url,omitempty"`
	UnsubscribePost bool   `json:"unsubscribe_post,omitempty"`
	// ReceiptRequested marks a read-receipt request (Disposition-Notification-To,
	// RFC 3798); ReceiptTo is the address the sender wants the receipt sent to.
	ReceiptRequested bool   `json:"receipt_requested,omitempty"`
	ReceiptTo        string `json:"receipt_to,omitempty"`
	// Recall is set when the message is a recall notice (Outlook-style
	// X-MS-Recall), linking to the original message by Message-ID.
	Recall *RecallInfo `json:"recall,omitempty"`
	// BurnAfterMinutes marks a burn-after-read (阅后即焚) message: the reader
	// shows the body once and flags it $BurnRead. 0 = normal message.
	BurnAfterMinutes int `json:"burn_after_minutes,omitempty"`
	// Category is the deterministic auto-classification (work/social/
	// newsletter/shopping/finance/other) derived from sender and headers.
	Category string `json:"category,omitempty"`
	// Invitation is the parsed text/calendar iTIP payload (meeting
	// REQUEST/REPLY/CANCEL), nil for ordinary messages.
	Invitation *Invitation `json:"invitation,omitempty"`
}

// RecallInfo identifies the original message a recall notice refers to.
type RecallInfo struct {
	MessageID string `json:"message_id"` // raw RFC 5322 Message-ID, e.g. "<abc@example.com>"
	Subject   string `json:"subject"`
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

// Client is an IMAP gateway. Each operation borrows a connection from the
// per-account pool (authenticated with a per-session temp token, never the
// user's password) so hot webmail requests skip the dial/TLS/login round trip;
// the pool falls back to a fresh connection exactly like the old stateless
// behaviour when nothing is reusable.
type Client struct {
	IMAPAddr  string // host:port, e.g. gateway:1143
	SMTPAddr  string // host:port, e.g. gateway:1587
	SieveAddr string // host:port, e.g. gateway:4190

	// dial, when set by With(), overrides the gateway target for every
	// operation on that client copy (external aggregated accounts).
	dial Dial

	// msgIDToUID caches the most recent (folder, Message-ID → UID) resolutions so
	// the reverse lookup served by UIDByMessageID is O(1) for hot messages
	// instead of scanning the whole mailbox every time.
	msgIDToUID msgIDCache

	// pool reuses authenticated IMAP connections per account. nil disables
	// pooling (defensive zero-value Client), keeping the stateless gateway.
	pool *poolRegistry

	// insecureTLS relaxes certificate verification for EXTERNAL aggregated
	// accounts only (opt-in, FETCH_INSECURE). Internal engine links are
	// trusted by deployment; external servers must verify by default or a
	// network attacker can harvest user credentials and mail.
	insecureTLS bool
}

// New creates a mail gateway client.
func New(imapAddr, smtpAddr, sieveAddr string) *Client {
	return &Client{
		IMAPAddr:  imapAddr,
		SMTPAddr:  smtpAddr,
		SieveAddr: sieveAddr,
		pool:      newPoolRegistry(),
	}
}

// SetInsecureTLS opts external aggregated-account dials out of TLS
// certificate verification (chainable). Internal links are unaffected.
func (c *Client) SetInsecureTLS(v bool) *Client {
	c.insecureTLS = v
	return c
}

func (c *Client) tlsConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // internal connections only
}

// openIMAP returns an authenticated IMAP connection. Internal accounts get a
// pooled connection (reused across requests); external (aggregated) accounts
// dial their own server with the stored credentials, unpooled.
func (c *Client) openIMAP(email, token string) (*pooledConn, error) {
	if c.dial.External {
		cli, err := c.openExternalIMAP(c.dial)
		if err != nil {
			return nil, err
		}
		return &pooledConn{Client: cli}, nil
	}
	if c.pool == nil {
		cli, err := c.dialIMAP(email, token)
		if err != nil {
			return nil, err
		}
		return &pooledConn{Client: cli}, nil
	}
	return c.pool.acquire(c, email, token)
}

// dialIMAP dials the gateway, upgrades to TLS when required and logs in.
func (c *Client) dialIMAP(email, token string) (*client.Client, error) {
	cli, err := client.Dial(c.IMAPAddr)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	if err := cli.StartTLS(c.tlsConfig()); err != nil {
		// MAILEZ_TLS=off deployments serve plaintext on the internal proxy
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
	// matches the other folders' Title-case names. The prefix of nested paths
	// ("INBOX/Sub") is normalized too: without it the sidebar would build a
	// separate "INBOX" tree node next to "Inbox" and show the inbox twice.
	// Every IMAP call later normalizes back via inboxName(). Wire names are
	// modified UTF-7 (RFC 3501 §5.1.3); decode so non-ASCII folder names are
	// not shown as "&XfJT0ZAB-" — the engine accepts raw UTF-8 on the way
	// back, so SELECT/CREATE can keep using the decoded spelling.
	for m := range mailboxes {
		folders = append(folders, inboxPath(imaputf7.FolderName(m.Name)))
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
	return c.ListMessagesSorted(email, token, folder, page, "date", "desc")
}

// ListMessagesSorted returns a page of messages ordered by the requested
// field ("date" keeps the fast newest-first path; from/subject/size fetch the
// whole folder and sort in Go, then slice the page so pagination stays
// correct across the ordering).
func (c *Client) ListMessagesSorted(email, token, folder string, page int, sortBy, dir string) ([]Message, int, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, 0, err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, folder, true)
	if err != nil {
		return nil, 0, fmt.Errorf("imap select %q: %w", folder, err)
	}
	if mbox.Messages == 0 || page < 0 {
		return []Message{}, int(mbox.Messages), nil
	}
	if sortBy != "" && sortBy != "date" {
		return c.listAllSorted(cli, folder, mbox.Messages, page, sortBy, dir)
	}
	if dir == "asc" {
		// 最旧优先: the fastest path always returns newest-first, so the
		// ascending page is fetched from the start of the mailbox instead.
		return c.listDateAsc(cli, mbox.Messages, page)
	}

	skip := uint32(page) * PageSize
	if skip >= mbox.Messages {
		return []Message{}, int(mbox.Messages), nil
	}
	end := mbox.Messages - skip
	start := uint32(1)
	if end > PageSize {
		start = end - PageSize + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, end)

	// Headers are fetched alongside the envelope so each row can be tagged
	// with a deterministic category (newsletter detection needs the
	// List-Unsubscribe header).
	headerSection := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier}}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, headerSection.FetchItem()}, messages)
	}()

	var out []Message
	for msg := range messages {
		m := envelopeToMessage(msg)
		m.Category = classifyFetched(msg, headerSection)
		out = append(out, m)
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

// listDateAsc returns one page of the oldest messages (date ascending). The
// mailbox stores messages newest-last on disk, so the ascending window is the
// first PageSize sequences of the folder.
func (c *Client) listDateAsc(cli *pooledConn, total uint32, page int) ([]Message, int, error) {
	skip := uint32(page) * PageSize
	if skip >= total {
		return []Message{}, int(total), nil
	}
	start := skip + 1
	end := skip + PageSize
	if end > total {
		end = total
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, end)

	headerSection := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier}}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, headerSection.FetchItem()}, messages)
	}()

	var out []Message
	for msg := range messages {
		m := envelopeToMessage(msg)
		m.Category = classifyFetched(msg, headerSection)
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, 0, fmt.Errorf("imap fetch: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out, int(total), nil
}

// conversationWindow bounds how many recent messages are scanned to build
// the conversation list, matching the thread-metadata window.
const conversationWindow = 300

// ListConversationsSorted returns one page of conversations (Gmail-style):
// messages sharing a thread id collapse into a single row represented by the
// newest member, carrying aggregated unread/flagged state, the distinct
// senders and the member count. Rows are ordered by the requested field of
// the representative (date desc = newest conversation first).
func (c *Client) ListConversationsSorted(email, token, folder string, page int, sortBy, dir string) ([]Message, int, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, 0, err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, folder, true)
	if err != nil {
		return nil, 0, fmt.Errorf("imap select %q: %w", folder, err)
	}
	if mbox.Messages == 0 || page < 0 {
		return []Message{}, int(mbox.Messages), nil
	}
	start := uint32(1)
	if mbox.Messages > conversationWindow {
		start = mbox.Messages - conversationWindow + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, mbox.Messages)
	headerSection := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier}}
	messages := make(chan *imap.Message, 50)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, imap.FetchRFC822Size, headerSection.FetchItem()}, messages)
	}()

	groups := map[string][]Message{}
	for msg := range messages {
		m := envelopeToMessage(msg)
		m.ThreadID = threadID(m.Subject)
		m.Category = classifyFetched(msg, headerSection)
		key := m.ThreadID
		if key == "" {
			// Messages without a subject never group; keep them as singletons.
			key = "uid:" + strconv.FormatUint(uint64(msg.Uid), 10)
		}
		groups[key] = append(groups[key], m)
	}
	if err := <-done; err != nil {
		return nil, 0, fmt.Errorf("imap fetch: %w", err)
	}

	reps := make([]Message, 0, len(groups))
	for _, members := range groups {
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].Date.Equal(members[j].Date) {
				return members[i].UID < members[j].UID
			}
			return members[i].Date.Before(members[j].Date)
		})
		rep := members[len(members)-1] // newest member represents the row
		rep.ThreadCount = len(members)
		rep.ThreadLatest = true
		rep.ThreadUnread = false
		rep.ThreadFlagged = false
		senders := map[string]bool{}
		for i := range members {
			if !slices.Contains(members[i].Flags, imap.SeenFlag) {
				rep.ThreadUnread = true
			}
			if slices.Contains(members[i].Flags, imap.FlaggedFlag) {
				rep.ThreadFlagged = true
			}
			if len(members[i].From) > 0 {
				n := members[i].From[0].Name
				if n == "" {
					n = members[i].From[0].Email
				}
				if n != "" && !senders[n] {
					senders[n] = true
					rep.ThreadSenders = append(rep.ThreadSenders, n)
				}
			}
		}
		reps = append(reps, rep)
	}

	lessAsc := func(i, j int) bool {
		switch sortBy {
		case "from":
			return senderKey(reps[i]) < senderKey(reps[j])
		case "subject":
			return strings.ToLower(reps[i].Subject) < strings.ToLower(reps[j].Subject)
		case "size":
			return reps[i].Size < reps[j].Size
		default:
			return reps[i].Date.Before(reps[j].Date)
		}
	}
	sort.SliceStable(reps, lessAsc)
	if dir == "desc" {
		for i, j := 0, len(reps)-1; i < j; i, j = i+1, j-1 {
			reps[i], reps[j] = reps[j], reps[i]
		}
	}

	total := len(reps)
	skip := page * PageSize
	if skip >= total {
		return []Message{}, total, nil
	}
	end := skip + PageSize
	if end > total {
		end = total
	}
	return reps[skip:end], total, nil
}

func senderKey(m Message) string {
	if len(m.From) == 0 {
		return ""
	}
	n := m.From[0].Name
	if n == "" {
		n = m.From[0].Email
	}
	return strings.ToLower(n)
}

// listAllSorted fetches every message of the folder, sorts by the requested
// field and returns the requested page.
func (c *Client) listAllSorted(cli *pooledConn, folder string, total uint32, page int, sortBy, dir string) ([]Message, int, error) {
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, total)
	headerSection := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier}}
	messages := make(chan *imap.Message, 50)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, imap.FetchRFC822Size, headerSection.FetchItem()}, messages)
	}()
	var out []Message
	for msg := range messages {
		m := envelopeToMessage(msg)
		m.Category = classifyFetched(msg, headerSection)
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, 0, fmt.Errorf("imap fetch: %w", err)
	}
	less := func(i, j int) bool {
		switch sortBy {
		case "from":
			ai, bi := out[i].From, out[j].From
			an, bn := "", ""
			if len(ai) > 0 {
				an = ai[0].Name
				if an == "" {
					an = ai[0].Email
				}
			}
			if len(bi) > 0 {
				bn = bi[0].Name
				if bn == "" {
					bn = bi[0].Email
				}
			}
			return strings.ToLower(an) < strings.ToLower(bn)
		case "subject":
			return strings.ToLower(out[i].Subject) < strings.ToLower(out[j].Subject)
		case "size":
			return out[i].Size < out[j].Size
		default:
			return out[i].Date.After(out[j].Date)
		}
	}
	sort.SliceStable(out, less)
	if dir == "desc" {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	start := page * PageSize
	if start >= len(out) {
		return []Message{}, len(out), nil
	}
	end := start + PageSize
	if end > len(out) {
		end = len(out)
	}
	return out[start:end], len(out), nil
}

// ListAllMessages returns envelope + flags + size for every message in the
// folder (no bodies). ActiveSync uses it to snapshot a collection for
// incremental sync; order is the mailbox order.
func (c *Client) ListAllMessages(email, token, folder string) ([]Message, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, folder, true)
	if err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}
	if mbox.Messages == 0 {
		return []Message{}, nil
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)
	headerSection := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier}}
	messages := make(chan *imap.Message, 50)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, imap.FetchRFC822Size, headerSection.FetchItem()}, messages)
	}()
	var out []Message
	for msg := range messages {
		out = append(out, envelopeToMessage(msg))
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	return out, nil
}

// FolderStat returns the SELECT counters for a folder (no body data).
func (c *Client) FolderStat(email, token, folder string) (FolderStat, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return FolderStat{}, err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, folder, true)
	if err != nil {
		return FolderStat{}, fmt.Errorf("imap select %q: %w", folder, err)
	}
	return FolderStat{Messages: mbox.Messages, Unseen: mbox.Unseen, UidNext: mbox.UidNext}, nil
}

// GetMessage returns a full message body by UID.
func (c *Client) GetMessage(email, token, folder string, uid uint32) (*Message, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	if _, err := c.selectFolder(cli, folder, true); err != nil {
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
		raw, err := io.ReadAll(body)
		if err == nil {
			applyBody(&out, raw)
		}
	}
	return &out, nil
}

// applyBody parses the raw RFC 5322 source into the message's text/html
// bodies, attachments, invitation and header-derived flags. Shared by the
// detail fetch and the conversation (thread) fetch.
func applyBody(out *Message, raw []byte) {
	out.UnsubscribeURL, out.UnsubscribePost = parseUnsubscribe(raw)
	out.ReceiptRequested, out.ReceiptTo = parseReceiptRequest(raw)
	out.Recall = parseRecallNotice(raw)
	out.BurnAfterMinutes = parseBurnAfter(raw)
	if textBody, htmlBody, attachments, inv, err := extractBody(bytes.NewReader(raw)); err == nil {
		out.TextBody = textBody
		out.HTMLBody = htmlBody
		out.Attachments = attachments
		out.Invitation = inv
	}
}

// parseBurnAfter reads X-Mailez-Burn-After (minutes) from a raw message.
func parseBurnAfter(raw []byte) int {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(msg.Header.Get("X-Mailez-Burn-After")))
	if err != nil || n <= 0 {
		return 0
	}
	if n > 7*24*60 {
		n = 7 * 24 * 60
	}
	return n
}

// parseReceiptRequest reads Disposition-Notification-To (RFC 3798) from the
// raw message headers.
func parseReceiptRequest(raw []byte) (requested bool, to string) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return false, ""
	}
	to = strings.TrimSpace(msg.Header.Get("Disposition-Notification-To"))
	return to != "", to
}

// parseRecallNotice detects an Outlook-style recall notice and returns the
// original message id it targets.
func parseRecallNotice(raw []byte) *RecallInfo {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(msg.Header.Get("X-MS-Recall")), "yes") {
		return nil
	}
	return &RecallInfo{
		MessageID: strings.TrimSpace(msg.Header.Get("X-MS-Recall-MessageID")),
		Subject:   strings.TrimSpace(msg.Header.Get("X-MS-Recall-Subject")),
	}
}

// parseUnsubscribe reads the List-Unsubscribe / List-Unsubscribe-Post headers
// (RFC 2369 / RFC 8058) from a raw message. Angle-bracketed URLs may be comma
// separated; an https entry wins over mailto because the backend can trigger
// it server-side without a mail round-trip.
func parseUnsubscribe(raw []byte) (url string, post bool) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", false
	}
	for _, candidate := range strings.Split(msg.Header.Get("List-Unsubscribe"), ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimSuffix(strings.TrimPrefix(candidate, "<"), ">")
		switch {
		case strings.HasPrefix(candidate, "https://"):
			if url == "" || !strings.HasPrefix(url, "https://") {
				url = candidate
			}
		case strings.HasPrefix(candidate, "mailto:") && url == "":
			url = candidate
		}
	}
	return url, strings.EqualFold(strings.TrimSpace(msg.Header.Get("List-Unsubscribe-Post")), "One-Click")
}

// GetRaw returns the full RFC 822 source of a message, for the "view raw"
// feature. Fetching the whole body works through the same BodySectionName the
// detail view uses.
func (c *Client) GetRaw(email, token, folder string, uid uint32) (string, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return "", err
	}
	defer cli.Logout()

	if _, err := c.selectFolder(cli, folder, true); err != nil {
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
		Size:  int64(msg.Size),
	}
	if msg.Envelope != nil {
		out.Subject = msg.Envelope.Subject
		out.Date = msg.Envelope.Date
		out.From = addresses(msg.Envelope.From)
		out.To = addresses(msg.Envelope.To)
		out.Cc = addresses(msg.Envelope.Cc)
		// Bcc is only ever set on the owner's own Drafts copy; mapping it
		// lets the draft editor restore the blind recipients on reopen.
		out.Bcc = addresses(msg.Envelope.Bcc)
		out.ID = EncodeMessageID(msg.Envelope.MessageId)
	}
	// JSON contract: from/to/flags are arrays, never null. A message whose
	// ENVELOPE carries no From (e.g. system notices) would otherwise
	// serialize a nil slice as null and crash frontend row rendering at
	// message.from[0]. addresses() already guarantees from/to/cc; flags can
	// still be nil when the FETCH response omits the FLAGS item.
	if out.Flags == nil {
		out.Flags = []string{}
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
	// Start from an allocated (non-nil) slice so an empty address list
	// serializes as [] instead of null (see envelopeToMessage).
	out := make([]Address, 0, len(list))
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
func extractBody(r io.Reader) (text, html string, attachments []Attachment, inv *Invitation, err error) {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return "", "", nil, nil, err
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
			if strings.HasPrefix(partType, "text/calendar") && inv == nil {
				b, _ := io.ReadAll(io.LimitReader(part, 1<<20))
				inv = ParseInvitation(decodeBodyBytes(b, part.Header.Get("Content-Transfer-Encoding")))
			}
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
		} else if strings.HasPrefix(mt, "text/calendar") {
			inv = ParseInvitation(decodeBodyBytes(b, encoding))
		}
	}
	return text, html, attachments, inv, nil
}

// decodeBodyBytes is decodeBody returning []byte (for ICS parsing).
func decodeBodyBytes(b []byte, encoding string) []byte {
	switch strings.ToLower(encoding) {
	case "base64":
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b))); err == nil {
			return decoded
		}
	case "quoted-printable":
		if r := quotedprintable.NewReader(bytes.NewReader(b)); r != nil {
			if decoded, err := io.ReadAll(r); err == nil {
				return decoded
			}
		}
	}
	return b
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
