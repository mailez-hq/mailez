package mail

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap"

	"mailez/backend/internal/mail/imaputf7"
)

// SetFlag adds or removes an IMAP flag (\Seen, \Flagged, ...) by UID.
func (c *Client) SetFlag(email, token, folder string, uid uint32, flag string, value bool) error {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	if _, err := c.selectFolder(cli, folder, false); err != nil {
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

// MarkAllRead marks every message in a folder as \Seen (one UID batch).
func (c *Client) MarkAllRead(email, token, folder string) error {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, folder, false)
	if err != nil {
		return fmt.Errorf("imap select %q: %w", folder, err)
	}
	if mbox.Messages == 0 || mbox.Unseen == 0 {
		return nil
	}
	// Fetch the UIDs of unseen messages so the store batch only touches the
	// messages that need the flag (cheap for folders with few unread).
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)
	messages := make(chan *imap.Message, 32)
	done := make(chan error, 1)
	go func() { done <- cli.Fetch(seqset, []imap.FetchItem{imap.FetchUid, imap.FetchFlags}, messages) }()
	uids := new(imap.SeqSet)
	for msg := range messages {
		seen := false
		for _, f := range msg.Flags {
			if strings.EqualFold(f, imap.SeenFlag) {
				seen = true
				break
			}
		}
		if !seen {
			uids.AddNum(msg.Uid)
		}
	}
	if err := <-done; err != nil {
		return fmt.Errorf("imap fetch: %w", err)
	}
	if uids.Empty() {
		return nil
	}
	if err := cli.UidStore(uids, imap.AddFlags, []interface{}{imap.RawString(`\Seen`)}, nil); err != nil {
		return fmt.Errorf("imap store seen: %w", err)
	}
	return nil
}

// SetFlags adds and removes IMAP flags/keywords by UID in one round trip.
func (c *Client) SetFlags(email, token, folder string, uid uint32, add, remove []string) error {
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	if _, err := c.selectFolder(cli, folder, false); err != nil {
		return fmt.Errorf("imap select %q: %w", folder, err)
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	if len(add) > 0 {
		vals := make([]interface{}, 0, len(add))
		for _, f := range add {
			vals = append(vals, imap.RawString(f))
		}
		if err := cli.UidStore(seqset, imap.AddFlags, vals, nil); err != nil {
			return fmt.Errorf("imap add flags: %w", err)
		}
	}
	if len(remove) > 0 {
		vals := make([]interface{}, 0, len(remove))
		for _, f := range remove {
			vals = append(vals, imap.RawString(f))
		}
		if err := cli.UidStore(seqset, imap.RemoveFlags, vals, nil); err != nil {
			return fmt.Errorf("imap remove flags: %w", err)
		}
	}
	return nil
}

// MoveMany relocates messages by UID to another folder, creating the target
// mailbox (Archive, Trash, Junk, ...) when it does not exist yet.
func (c *Client) MoveMany(email, token, folder string, uids []uint32, destination string) error {
	if len(uids) == 0 {
		return nil
	}
	destination = inboxName(destination)
	if err := c.EnsureMailbox(email, token, destination); err != nil {
		return err
	}
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	if _, err := c.selectFolder(cli, folder, false); err != nil {
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
	name = inboxName(name)
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
		if _, err := c.selectFolder(cli, f, false); err != nil {
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
			// Legacy literal "Inbox/..." mailboxes are not reachable through
			// the canonical spelling; retry with the exact server name.
			if st, err = cli.Status(f, []imap.StatusItem{imap.StatusUnseen}); err != nil {
				continue
			}
		}
		// Key by the display spelling so the sidebar badges line up with the
		// folder tree, which uses the canonical "Inbox" prefix. Decode the
		// wire name (modified UTF-7) so the keys match the decoded folder
		// tree; the wire call above used the raw server spelling.
		out[inboxPath(imaputf7.FolderName(f))] = int(st.Unseen)
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

// IsSystemFolder reports whether name is one of the engine-created system
// mailboxes. Only the special INBOX is matched case-insensitively; the rest
// must match the canonical Title-case spelling the engine creates, so a
// user-created folder literally named "sent" or "trash" is not accidentally
// protected (and therefore impossible to rename or delete).
func IsSystemFolder(name string) bool {
	switch inboxName(name) {
	case "INBOX", "Sent", "Drafts", "Trash", "Archive", "Junk":
		return true
	}
	return false
}

// CreateFolder creates a new mailbox, failing when it already exists so the
// UI can surface the conflict instead of silently succeeding.
func (c *Client) CreateFolder(email, token, name string) error {
	name = inboxName(name)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if err := cli.Create(name); err != nil {
		return fmt.Errorf("imap create %q: %w", name, err)
	}
	return nil
}

// RenameFolder renames a mailbox. Renaming the reserved INBOX is refused.
func (c *Client) RenameFolder(email, token, oldName, newName string) error {
	if strings.EqualFold(oldName, "inbox") {
		return errors.New("cannot rename INBOX")
	}
	origOld := oldName
	oldName = inboxName(oldName)
	newName = inboxName(newName)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if err := cli.Rename(oldName, newName); err != nil {
		// Legacy literal "Inbox/..." mailbox: retry with the exact server
		// spelling so renames of folders created before the normalization
		// fix keep working.
		if origOld != oldName {
			if err2 := cli.Rename(origOld, newName); err2 == nil {
				return nil
			}
		}
		return fmt.Errorf("imap rename %q: %w", oldName, err)
	}
	return nil
}

// DeleteFolder deletes a mailbox. The reserved INBOX cannot be deleted.
func (c *Client) DeleteFolder(email, token, name string) error {
	if strings.EqualFold(name, "inbox") {
		return errors.New("cannot delete INBOX")
	}
	name = inboxName(name)
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if err := cli.Delete(name); err != nil {
		if err2 := cli.Delete(inboxPath(name)); err2 == nil {
			return nil
		}
		return fmt.Errorf("imap delete %q: %w", name, err)
	}
	return nil
}

// ClearFolder marks every message in a mailbox as \Deleted and expunges it.
func (c *Client) ClearFolder(email, token, name string) error {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	mbox, err := c.selectFolder(cli, name, false)
	if err != nil {
		return fmt.Errorf("imap select %q: %w", name, err)
	}
	if mbox.Messages == 0 {
		return nil
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)
	if err := cli.Store(seqset, imap.AddFlags, []interface{}{imap.RawString("\\Deleted")}, nil); err != nil {
		return fmt.Errorf("imap store %q: %w", name, err)
	}
	deleted := make(chan uint32, 1)
	if err := cli.Expunge(deleted); err != nil {
		return fmt.Errorf("imap expunge %q: %w", name, err)
	}
	return nil
}
