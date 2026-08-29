package mail

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
)

const (
	// SnoozeFlag marks a message as snoozed (hidden until its wake-up time).
	SnoozeFlag = "$Snoozed"
	// SnoozeUntilPrefix encodes the wake-up time as $SnoozedUntil-<unix>.
	SnoozeUntilPrefix = "$SnoozedUntil-"
)

// SnoozedMessage is a message currently snoozed, with its wake-up time.
type SnoozedMessage struct {
	Message
	Until time.Time `json:"until"`
}

// Snooze pauses or wakes a message. With a non-nil until the message is
// snoozed (marked seen so it leaves the unread counts) and hidden behind the
// SnoozeFlag keyword; with nil it is returned to the inbox unread and any
// snooze keywords are stripped.
func (c *Client) Snooze(email, token, folder string, uid uint32, until *time.Time) error {
	if until != nil {
		add := []string{SnoozeFlag, SnoozeUntilPrefix + strconv.FormatInt(until.Unix(), 10), imap.SeenFlag}
		return c.SetFlags(email, token, folder, uid, add, nil)
	}
	// Un-snooze: fetch the message first so every snooze keyword is stripped.
	msg, err := c.GetMessage(email, token, folder, uid)
	if err != nil {
		return err
	}
	// Use the actual keyword case from the stored flags (IMAP servers and
	// stores canonicalize keywords to lowercase) so the removal matches.
	remove := []string{imap.SeenFlag}
	remove = append(remove, snoozeKeywords(msg.Flags)...)
	return c.SetFlags(email, token, folder, uid, nil, remove)
}

// SnoozedMessages lists snoozed messages across every mailbox, newest first.
// Messages whose wake-up time has passed are resurfaced first: their snooze
// keywords are stripped and they are returned to unread, so they reappear in
// the inbox as due. Only the still-snoozed messages are returned.
func (c *Client) SnoozedMessages(email, token string) ([]SnoozedMessage, error) {
	folders, err := c.ListFolders(email, token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	// Non-nil empty slice: JSON null (nil slice) violates the array contract.
	out := make([]SnoozedMessage, 0)
	for _, folder := range folders {
		msgs, err := c.SearchMessagesSpec(email, token, folder, SearchQuery{Labels: []string{SnoozeFlag}})
		if err != nil {
			continue
		}
		for i := range msgs {
			until, ok := snoozeUntilFromFlags(msgs[i].Flags)
			if !ok {
				// Keyword-only snooze (no until): never auto-resurface.
				until = now.Add(365 * 24 * time.Hour)
			}
			if !until.After(now) {
				_ = c.Snooze(email, token, folder, msgs[i].UID, nil)
				continue
			}
			// Tag the source folder (same convention as search-all): the
			// snoozed view merges every mailbox, and uids are only unique
			// per folder - without the tag, wake-ups target the wrong
			// folder and list rows collide on duplicate keys.
			msgs[i].Folder = folder
			out = append(out, SnoozedMessage{Message: msgs[i], Until: until})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Until.Before(out[j].Until) })
	return out, nil
}

// snoozeUntilFromFlags extracts the wake-up time from a message's keywords.
func snoozeUntilFromFlags(flags []string) (time.Time, bool) {
	prefix := strings.ToLower(SnoozeUntilPrefix)
	for _, f := range flags {
		lower := strings.ToLower(f)
		if strings.HasPrefix(lower, prefix) {
			if n, err := strconv.ParseInt(strings.TrimPrefix(lower, prefix), 10, 64); err == nil {
				return time.Unix(n, 0), true
			}
		}
	}
	return time.Time{}, false
}

// snoozeKeywords lists the snooze keywords present on a message so they can be
// stripped together.
func snoozeKeywords(flags []string) []string {
	var out []string
	flag := strings.ToLower(SnoozeFlag)
	prefix := strings.ToLower(SnoozeUntilPrefix)
	for _, f := range flags {
		lower := strings.ToLower(f)
		if lower == flag || strings.HasPrefix(lower, prefix) {
			out = append(out, f)
		}
	}
	return out
}
