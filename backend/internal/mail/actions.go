package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap"
)

// SetFlag adds or removes an IMAP flag (\Seen, \Flagged, ...) by UID.
func (c *Client) SetFlag(email, token, folder string, uid uint32, flag string, value bool) error {
	folder = inboxName(folder)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, false); err != nil {
		return fmt.Errorf("imap select %q: %w", folder, err)
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	var op imap.StoreItem = imap.RemoveFlags
	if value {
		op = imap.AddFlags
	}
	// value must be []interface{} of RawString flags for go-imap.
	if err := cli.UidStore(seqset, op, []interface{}{imap.RawString(flag)}, nil); err != nil {
		return fmt.Errorf("imap store: %w", err)
	}
	return nil
}

// MoveMany relocates messages by UID to another folder, creating the target
// mailbox (Archive, Trash, Junk, ...) when it does not exist yet.
func (c *Client) MoveMany(email, token, folder string, uids []uint32, destination string) error {
	if len(uids) == 0 {
		return nil
	}
	folder = inboxName(folder)
	destination = inboxName(destination)
	if err := c.EnsureMailbox(email, token, destination); err != nil {
		return err
	}
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, false); err != nil {
		return fmt.Errorf("imap select %q: %w", folder, err)
	}
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	if err := cli.UidMove(seqset, destination); err != nil {
		return fmt.Errorf("imap move: %w", err)
	}
	return nil
}

// Move relocates a single message by UID to another folder.
func (c *Client) Move(email, token, folder string, uid uint32, destination string) error {
	return c.MoveMany(email, token, folder, []uint32{uid}, destination)
}

// Delete moves a message to Trash (creating it if needed).
func (c *Client) Delete(email, token, folder string, uid uint32) error {
	return c.Move(email, token, folder, uid, "Trash")
}

// EnsureMailbox creates the mailbox if it does not exist.
func (c *Client) EnsureMailbox(email, token, name string) error {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() { done <- cli.List("", name, mailboxes) }()
	found := false
	for range mailboxes {
		found = true
	}
	if err := <-done; err != nil {
		return fmt.Errorf("imap list: %w", err)
	}
	if found {
		return nil
	}
	return cli.Create(name)
}

// ReplaceKeyword renames an IMAP keyword across every mailbox: messages
// carrying oldKw get newKw added (when non-empty) and oldKw removed. Label
// rename/delete use this so tag state stays consistent everywhere.
func (c *Client) ReplaceKeyword(email, token, oldKw, newKw string) error {
	folders, err := c.ListFolders(email, token)
	if err != nil {
		return err
	}
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	for _, f := range folders {
		if _, err := cli.Select(inboxName(f), false); err != nil {
			continue // unreadable mailbox: skip rather than abort the sweep
		}
		uids, err := cli.UidSearch(&imap.SearchCriteria{WithFlags: []string{oldKw}})
		if err != nil || len(uids) == 0 {
			continue
		}
		seqset := new(imap.SeqSet)
		for _, uid := range uids {
			seqset.AddNum(uid)
		}
		if newKw != "" {
			if err := cli.UidStore(seqset, imap.AddFlags, []interface{}{imap.RawString(newKw)}, nil); err != nil {
				return fmt.Errorf("imap store %q: %w", newKw, err)
			}
		}
		if err := cli.UidStore(seqset, imap.RemoveFlags, []interface{}{imap.RawString(oldKw)}, nil); err != nil {
			return fmt.Errorf("imap store %q: %w", oldKw, err)
		}
	}
	return nil
}

// UnseenCounts returns the number of unseen messages per mailbox, for the
// sidebar badges. Folders that fail STATUS are skipped.
func (c *Client) UnseenCounts(email, token string) (map[string]int, error) {
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

	out := make(map[string]int, len(folders))
	for _, f := range folders {
		st, err := cli.Status(inboxName(f), []imap.StatusItem{imap.StatusUnseen})
		if err != nil {
			continue
		}
		out[inboxName(f)] = int(st.Unseen)
	}
	return out, nil
}

// appendLiteral adapts a strings.Reader to the imap.Literal interface.
type appendLiteral struct{ *strings.Reader }

func (a appendLiteral) Len() int { return a.Reader.Len() }

// SaveDraft appends a message to Drafts with the \Draft flag. When replaceUID
// is non-zero the previous draft is removed first (auto-save keeps exactly one
// draft per compose session). The new message's UID is returned when known.
// to/cc keep the recipients and attachments keep the files, so reopening the
// draft restores the whole compose state.
func (c *Client) SaveDraft(email, token string, to, cc []string, subject, text, html string, attachments []Attachment, replaceUID uint32) (uint32, error) {
	if err := c.EnsureMailbox(email, token, "Drafts"); err != nil {
		return 0, err
	}
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return 0, err
	}
	defer cli.Logout()

	if _, err := cli.Select("Drafts", false); err != nil {
		return 0, fmt.Errorf("imap select drafts: %w", err)
	}
	if replaceUID > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(replaceUID)
		if err := cli.UidStore(seqset, imap.AddFlags, []interface{}{imap.RawString("\\Deleted")}, nil); err == nil {
			deleted := make(chan uint32, 1)
			_ = cli.Expunge(deleted)
		}
	}

	msg := BuildMessage(email, to, cc, subject, text, html, attachments)
	if err := cli.Append("Drafts", []string{"\\Draft"}, time.Now(), appendLiteral{strings.NewReader(msg)}); err != nil {
		return 0, fmt.Errorf("imap append draft: %w", err)
	}
	st, err := cli.Status("Drafts", []imap.StatusItem{imap.StatusUidNext})
	if err != nil || st == nil || st.UidNext <= 1 {
		return 0, nil // uid unknown; caller just saves again without replacing
	}
	return st.UidNext - 1, nil
}
