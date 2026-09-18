package mail

import (
	"fmt"

	"github.com/emersion/go-imap"
)

// uidExpungeCmd is one UID EXPUNGE command (RFC 4315). go-imap v1 ships no
// UID EXPUNGE, so it is driven through the same client.Execute extension hook
// as UID SORT and the ACL commands.
type uidExpungeCmd struct{ set *imap.SeqSet }

func (c *uidExpungeCmd) Command() *imap.Command {
	return &imap.Command{
		Name:      "UID",
		Arguments: []interface{}{imap.RawString("EXPUNGE"), c.set},
	}
}

// expungeSink swallows the "* n EXPUNGE" untagged responses of one UID
// EXPUNGE; the caller only cares whether the command succeeded.
type expungeSink struct{}

func (expungeSink) Handle(imap.Resp) error { return nil }

// PurgeMany permanently removes messages from a folder: they are flagged
// \Deleted and then UID EXPUNGE'd in one pass, so the storage behind them is
// released. This is the delete path for a folder that has no further place to
// move a message to (Trash).
//
// UID EXPUNGE rather than a plain EXPUNGE keeps the removal to the UIDs we
// just flagged: a bare EXPUNGE would also take messages another client flagged
// \Deleted concurrently. The engine advertises UIDPLUS.
func (c *Client) PurgeMany(email, token, folder string, uids []uint32) error {
	if len(uids) == 0 {
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
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	if err := cli.UidStore(seqset, imap.AddFlags, []interface{}{imap.RawString(`\Deleted`)}, nil); err != nil {
		return fmt.Errorf("imap store \\Deleted: %w", err)
	}
	status, err := cli.Execute(&uidExpungeCmd{set: seqset}, expungeSink{})
	if err != nil {
		return fmt.Errorf("imap uid expunge: %w", err)
	}
	if status != nil {
		if err := status.Err(); err != nil {
			return fmt.Errorf("imap uid expunge: %w", err)
		}
	}
	return nil
}

// Purge permanently removes a single message.
func (c *Client) Purge(email, token, folder string, uid uint32) error {
	return c.PurgeMany(email, token, folder, []uint32{uid})
}
