package mail

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
)

// msgIDCache is a small in-memory TTL cache keyed by "folder \x00 Message-ID"
// mapping to a resolved UID. It turns the reverse lookup in UIDByMessageID into
// an O(1) hit for hot messages. UIDs can shift when messages are expunged or
// moved, so entries expire after msgIDCacheTTL and are repopulated on the next
// full scan; a stale hit within the window is harmless because the reader
// revalidates against the folder when it fetches.
type msgIDCache struct {
	mu  sync.Mutex
	ttl time.Duration
	en  map[string]msgIDEntry
}

type msgIDEntry struct {
	uid    uint32
	minted time.Time
}

const msgIDCacheTTL = 5 * time.Minute

// get returns the cached UID for key if it is present and fresh.
func (mc *msgIDCache) get(key string) (uint32, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	e, ok := mc.en[key]
	if !ok {
		return 0, false
	}
	if time.Since(e.minted) > mc.ttl {
		delete(mc.en, key)
		return 0, false
	}
	return e.uid, true
}

// put stores the resolved UID for key. A nil/empty map is lazily initialized.
func (mc *msgIDCache) put(key string, uid uint32) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.en == nil {
		mc.en = make(map[string]msgIDEntry)
	}
	if mc.ttl == 0 {
		mc.ttl = msgIDCacheTTL
	}
	mc.en[key] = msgIDEntry{uid: uid, minted: time.Now()}
}

// Message ID helpers.
//
// The routable ID we expose for a message must be reversible back to the
// underlying IMAP UID without any local storage (this gateway is stateless
// across requests and nodes). We derive it from the RFC Message-ID header,
// which is guaranteed unique and immutable across mailboxes, and encode it
// URL-safe (base64url). Because Message-ID already carries a random token,
// the encoded form is unguessable (not enumerable), while decoding it lets us
// resolve the message back to its UID by scanning the folder's envelopes.

// inboxName canonicalizes the externally-facing folder name back to the
// protocol-reserved "INBOX". Per RFC 3501, "INBOX" is case-insensitive, but
// we normalize every spelling ("Inbox", "inbox") to the upper-case form so a
// single identity flows into every IMAP call regardless of what the client
// routing (URL, UI) happened to use.
func inboxName(folder string) string {
	if strings.EqualFold(folder, "inbox") {
		return "INBOX"
	}
	return folder
}

// EncodeMessageID wraps the raw Message-ID (e.g. "<abc123@example.com>") into
// a URL-safe opaque id. A missing/empty Message-ID yields "." so callers can
// still build a degenerate (non-stable) route for that rare case.
func EncodeMessageID(msgID string) string {
	msgID = strings.TrimSpace(msgID)
	if msgID == "" {
		return "."
	}
	return base64.RawURLEncoding.EncodeToString([]byte(msgID))
}

// DecodeMessageID undoes EncodeMessageID. It returns an empty string (and no
// error) for the degenerate "." form.
func DecodeMessageID(id string) (string, error) {
	if id == "." || id == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UIDByMessageID resolves a routable id back to the IMAP UID of the message
// whose Message-ID header matches. It iterates the mailbox envelopes rather
// than issuing a SEARCH HEADER command: many servers (and this stack's Dovecot
// setup) do not index arbitrary headers, so SEARCH "Message-ID" can silently
// return nothing even when the message is present. Comparing the parsed
// Message-ID directly is index-independent and deterministic.
func (c *Client) UIDByMessageID(email, token, folder, id string) (uint32, error) {
	folder = inboxName(folder)
	msgID, err := DecodeMessageID(id)
	if err != nil {
		return 0, fmt.Errorf("decode message id: %w", err)
	}
	if msgID == "" {
		return 0, errors.New("message id unavailable")
	}

	// Normalize both sides by stripping RFC 822 delimiter brackets so the
	// comparison is robust to whether the envelope carries them or not.
	want := strings.Trim(strings.TrimSpace(msgID), "<>")
	key := folder + "\x00" + want

	if uid, ok := c.msgIDToUID.get(key); ok {
		return uid, nil
	}

	cli, err := c.openIMAP(email, token)
	if err != nil {
		return 0, err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, true); err != nil {
		return 0, fmt.Errorf("imap select %q: %w", folder, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, 0) // 0 = last, selects the whole range per go-imap
	msgs := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, msgs)
	}()

	for m := range msgs {
		if m.Envelope == nil {
			continue
		}
		if strings.Trim(strings.TrimSpace(m.Envelope.MessageId), "<>") == want {
			c.msgIDToUID.put(key, m.Uid)
			return m.Uid, nil
		}
	}
	if err := <-done; err != nil {
		return 0, fmt.Errorf("imap fetch envelopes: %w", err)
	}
	return 0, errors.New("message not found")
}
