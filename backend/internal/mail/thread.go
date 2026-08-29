package mail

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

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
	return "s:" + n
}

// threadNode is one scanned envelope reduced to its threading identity.
type threadNode struct {
	uid       uint32
	msgid     string // normalized "<id@host>"; "" when the mail has none
	inReplyTo string // first In-Reply-To entry, normalized; "" when absent
	subject   string // subject fallback key ("" when degenerate)
	date      time.Time
}

// normalizeMsgID canonicalizes a Message-ID to the bracketed wire form.
func normalizeMsgID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "<") {
		s = "<" + s
	}
	if !strings.HasSuffix(s, ">") {
		s += ">"
	}
	return s
}

func envelopeThreadNode(msg *imap.Message) (threadNode, bool) {
	if msg.Envelope == nil {
		return threadNode{}, false
	}
	n := threadNode{
		uid:     msg.Uid,
		subject: threadID(msg.Envelope.Subject),
		date:    msg.Envelope.Date, // zero date sorts oldest — deterministic
	}
	if msg.Envelope.MessageId != "" {
		n.msgid = normalizeMsgID(msg.Envelope.MessageId)
	}
	if msg.Envelope.InReplyTo != "" {
		n.inReplyTo = normalizeMsgID(msg.Envelope.InReplyTo)
	}
	return n, true
}

// groupThreads derives one canonical thread key per scanned message.
//
// Policy (Gmail-faithful, backwards compatible): messages linked through
// In-Reply-To inside the window form one component keyed by the component's
// root message-id ("m:<id>"), so two genuinely separate conversations that
// merely share a subject no longer merge. Messages with no usable references
// keep the subject key ("s:<normalized>") that has always grouped them, and
// a reply whose parent fell outside the window degrades to the subject key
// instead of orphaning itself.
func groupThreads(nodes []threadNode) map[uint32]string {
	byMsgid := make(map[string]uint32, len(nodes))
	for _, n := range nodes {
		if n.msgid != "" {
			if _, ok := byMsgid[n.msgid]; !ok {
				byMsgid[n.msgid] = n.uid
			}
		}
	}

	// Union-find over uids. Roots never gain outgoing edges, so the parent
	// map stays a forest (mutual-reference cycles cannot form).
	parent := make(map[uint32]uint32, len(nodes))
	find := func(x uint32) uint32 {
		root := x
		for {
			p, ok := parent[root]
			if !ok || p == root {
				break
			}
			root = p
		}
		for {
			p, ok := parent[x]
			if !ok || p == x || p == root {
				break
			}
			parent[x] = root
			x = p
		}
		parent[root] = root
		return root
	}
	union := func(a, b uint32) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for _, n := range nodes {
		if n.inReplyTo == "" {
			continue
		}
		if p, ok := byMsgid[n.inReplyTo]; ok && p != n.uid {
			union(n.uid, p)
		}
	}

	groups := make(map[uint32][]threadNode)
	for _, n := range nodes {
		r := find(n.uid)
		groups[r] = append(groups[r], n)
	}

	keys := make(map[uint32]string, len(nodes))
	for _, members := range groups {
		key := componentKey(members)
		if key == "" {
			continue
		}
		for _, m := range members {
			keys[m.uid] = key
		}
	}
	return keys
}

// componentKey picks the canonical key for one component: the oldest member's
// message-id when the component is reply-linked, otherwise the subject
// fallback of its oldest member.
func componentKey(members []threadNode) string {
	sorted := append([]threadNode(nil), members...)
	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].date.Equal(sorted[j].date) {
			return sorted[i].date.Before(sorted[j].date)
		}
		return sorted[i].uid < sorted[j].uid
	})
	if len(members) > 1 {
		for _, m := range sorted {
			if m.msgid != "" {
				return "m:" + m.msgid
			}
		}
	}
	return sorted[0].subject
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

	nodes := make([]threadNode, 0, int(total-start+1))
	for msg := range messages {
		if n, ok := envelopeThreadNode(msg); ok {
			nodes = append(nodes, n)
		}
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap thread fetch: %w", err)
	}

	meta := &threadMeta{
		ids:    groupThreads(nodes),
		counts: map[string]int{},
		latest: map[string]uint32{},
	}
	for uid, tid := range meta.ids {
		meta.counts[tid]++
		if cur, ok := meta.latest[tid]; !ok || uid > cur {
			meta.latest[tid] = uid
		}
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
	meta, err := c.threadMeta(cli, mbox.Messages)
	if err != nil {
		return nil, err
	}

	// Non-nil empty slice: JSON null (nil slice) violates the array contract.
	out := make([]Message, 0)
	matched := make([]uint32, 0)
	for uid, t := range meta.ids {
		if t == tid {
			matched = append(matched, uid)
		}
	}
	if len(matched) == 0 {
		return out, nil
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i] < matched[j] })
	seqset := new(imap.SeqSet)
	for _, uid := range matched {
		seqset.AddNum(uid)
	}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	// Peek the body so opening a conversation never marks its other members
	// as read; only the message the user actually opened gets \Seen (through
	// the detail fetch).
	section := &imap.BodySectionName{Peek: true}
	go func() {
		done <- cli.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, section.FetchItem()}, messages)
	}()
	for msg := range messages {
		m := envelopeToMessage(msg)
		m.ThreadID = tid
		if body := msg.GetBody(section); body != nil {
			if raw, err := io.ReadAll(body); err == nil {
				applyBody(&m, raw)
			}
		}
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap thread fetch: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}
