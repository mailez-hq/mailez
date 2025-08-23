package mail

import (
	"fmt"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// ACLEntry pairs a mailbox identifier (a login or a group) with the rights
// string granted to it, as reported by GETACL (RFC 4314).
type ACLEntry struct {
	Identifier string `json:"identifier"`
	Rights     string `json:"rights"`
}

// aclCmd is an arbitrary IMAP ACL command. go-imap has no native ACL support,
// so we drive RFC 4314 through the extension hook in client.Execute.
type aclCmd struct {
	name string
	args []interface{}
}

func (c *aclCmd) Command() *imap.Command {
	return &imap.Command{Name: c.name, Arguments: c.args}
}

// aclCollector captures the untagged data responses produced by one command.
type aclCollector struct {
	responses []*imap.DataResp
}

func (c *aclCollector) Handle(resp imap.Resp) error {
	if data, ok := resp.(*imap.DataResp); ok {
		c.responses = append(c.responses, data)
	}
	return nil
}

// aclExec runs one ACL command and returns its untagged data responses.
func (c *Client) aclExec(cli *client.Client, name string, args ...interface{}) ([]*imap.DataResp, error) {
	h := &aclCollector{}
	if _, err := cli.Execute(&aclCmd{name: name, args: args}, h); err != nil {
		return nil, fmt.Errorf("imap %s: %w", name, err)
	}
	return h.responses, nil
}

// FolderACL returns every ACL entry for a folder (GETACL).
func (c *Client) FolderACL(email, token, folder string) ([]ACLEntry, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	resps, err := c.aclExec(cli, "GETACL", imap.RawString(inboxName(folder)))
	if err != nil {
		return nil, err
	}
	var entries []ACLEntry
	for _, resp := range resps {
		name, fields, ok := imap.ParseNamedResp(resp)
		if !ok || name != "ACL" || len(fields) < 2 {
			continue
		}
		// fields = [mailbox, id1, rights1, id2, rights2, ...]
		for i := 1; i+1 < len(fields); i += 2 {
			id, _ := fields[i].(string)
			rights, _ := fields[i+1].(string)
			if id != "" {
				entries = append(entries, ACLEntry{Identifier: id, Rights: rights})
			}
		}
	}
	return entries, nil
}

// MyRights returns the rights the authenticated user holds on a folder
// (MYRIGHTS), e.g. to hide the sharing UI when the mailbox is not shared.
func (c *Client) MyRights(email, token, folder string) (string, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return "", err
	}
	defer cli.Logout()

	resps, err := c.aclExec(cli, "MYRIGHTS", imap.RawString(inboxName(folder)))
	if err != nil {
		return "", err
	}
	for _, resp := range resps {
		name, fields, ok := imap.ParseNamedResp(resp)
		if !ok || name != "MYRIGHTS" || len(fields) < 2 {
			continue
		}
		if rights, ok := fields[len(fields)-1].(string); ok {
			return rights, nil
		}
	}
	return "", nil
}

// ListRights reports what is already granted to an identifier on a folder and
// which rights the caller may grant it (LISTRIGHTS).
func (c *Client) ListRights(email, token, folder, identifier string) (granted, available string, err error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return "", "", err
	}
	defer cli.Logout()

	resps, err := c.aclExec(cli, "LISTRIGHTS", imap.RawString(inboxName(folder)), imap.RawString(identifier))
	if err != nil {
		return "", "", err
	}
	for _, resp := range resps {
		name, fields, ok := imap.ParseNamedResp(resp)
		if !ok || name != "LISTRIGHTS" || len(fields) < 3 {
			continue
		}
		granted, _ = fields[2].(string)
		for _, f := range fields[3:] {
			if s, ok := f.(string); ok {
				available += s
			}
		}
		return granted, available, nil
	}
	return "", "", nil
}

// SetFolderACL grants or replaces the rights of an identifier on a folder
// (SETACL). An empty rights string removes every right.
func (c *Client) SetFolderACL(email, token, folder, identifier, rights string) error {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	_, err = c.aclExec(cli, "SETACL", imap.RawString(inboxName(folder)), imap.RawString(identifier), imap.RawString(rights))
	return err
}

// DeleteFolderACL removes every right of an identifier on a folder (DELETEACL).
func (c *Client) DeleteFolderACL(email, token, folder, identifier string) error {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return err
	}
	defer cli.Logout()

	_, err = c.aclExec(cli, "DELETEACL", imap.RawString(inboxName(folder)), imap.RawString(identifier))
	return err
}
