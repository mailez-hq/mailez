package mail

import (
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strings"

	"github.com/emersion/go-imap"
)

// threadWindow bounds how many recent envelopes are scanned to build thread
// metadata. Enough to group an active conversation without fetching a whole
// huge mailbox.
const threadWindow = 300

// fetcher is the subset of *client.Client used by threadMeta (kept narrow so
// the aggregation logic stays unit-testable).
type fetcher interface {
	Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error
}

// normalizeSubject lowercases, trims and strips repeated reply/forward
// prefixes so "Re: Re: Hello" and "FW: hello" group together.
func normalizeSubject(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for {
		orig := s
		for _, p := range []string{"re:", "fwd:", "fw:", "aw:", "antw:", "sv:", "vs:", "答复:", "回复:"} {
			if strings.HasPrefix(s, p) {
				s = strings.TrimSpace(s[len(p):])
			}
		}
		s = strings.Join(strings.Fields(s), " ")
		if s == orig {
			break
		}
	}
	return s
}

// threadID derives a stable, folder-local id from a subject. Empty (or
// degenerate) subjects yield "" so they are never grouped.
func threadID(subject string) string {
	n := normalizeSubject(subject)
	if n == "" {
		return ""
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(n))
	return fmt.Sprintf("%x", h.Sum64())
}

// threadMeta scans the most recent threadWindow envelopes of a selected
// mailbox and returns per-UID thread ids, per-thread counts and whether a UID
// is the newest message of its thread (UIDs are monotonic on Dovecot).
type threadMeta struct {
	ids    map[uint32]string
	counts map[string]int
	latest map[string]uint32
}

func (c *Client) threadMeta(cli fetcher, total uint32) (*threadMeta, error) {
	if total == 0 {
		return &threadMeta{
			ids:    map[uint32]string{},
			counts: map[string]int{},
			latest: map[string]uint32{},
		}, nil
	}
	start := uint32(1)
	if total > threadWindow {
		start = total - threadWindow + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, total)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid}, messages)
	}()

	meta := &threadMeta{
		ids:    map[uint32]string{},
		counts: map[string]int{},
		latest: map[string]uint32{},
	}
	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		tid := threadID(msg.Envelope.Subject)
		if tid == "" {
			continue
		}
		meta.ids[msg.Uid] = tid
		meta.counts[tid]++
		if cur, ok := meta.latest[tid]; !ok || msg.Uid > cur {
			meta.latest[tid] = msg.Uid
		}
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap thread fetch: %w", err)
	}
	return meta, nil
}

// Thread returns the full set of messages sharing a thread id within the
// recent window, oldest first, so the reading pane can walk a conversation.
func (c *Client) Thread(email, token, folder, tid string) ([]Message, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, folder, true)
	if err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}
	start := uint32(1)
	if mbox.Messages > threadWindow {
		start = mbox.Messages - threadWindow + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, mbox.Messages)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	// Peek the body so opening a conversation never marks its other members
	// as read; only the message the user actually opened gets \Seen (through
	// the detail fetch).
	section := &imap.BodySectionName{Peek: true}
	go func() {
		done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, section.FetchItem()}, messages)
	}()

	var out []Message
	for msg := range messages {
		if msg.Envelope != nil && threadID(msg.Envelope.Subject) == tid {
			m := envelopeToMessage(msg)
			m.ThreadID = tid
			if body := msg.GetBody(section); body != nil {
				if raw, err := io.ReadAll(body); err == nil {
					applyBody(&m, raw)
				}
			}
			out = append(out, m)
		}
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap thread fetch: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}
