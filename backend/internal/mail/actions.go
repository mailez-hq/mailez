package mail

import (
	"fmt"

	"github.com/emersion/go-imap"
)

// SetFlag adds or removes an IMAP flag (\Seen, \Flagged, ...) by UID.
func (c *Client) SetFlag(email, token, folder string, uid uint32, flag string, value bool) error {
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
