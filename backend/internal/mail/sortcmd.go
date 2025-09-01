package mail

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap"
)

// sortCmd is one UID SORT command (RFC 5256). go-imap v1 ships no SORT
// support, so it is driven through the same client.Execute extension hook
// as the ACL commands.
type sortCmd struct {
	spec string // parenthesized sort program, e.g. "(FROM)" or "(REVERSE SIZE)"
}

func (c *sortCmd) Command() *imap.Command {
	return &imap.Command{
		Name: "UID SORT",
		Arguments: []interface{}{
			imap.RawString(c.spec),
			imap.RawString("UTF-8"),
			imap.RawString("ALL"),
		},
	}
}

// sortCollector captures the "* SORT <uids>..." untagged responses of one
// UID SORT command.
type sortCollector struct {
	uids []uint32
}

func (s *sortCollector) Handle(resp imap.Resp) error {
	data, ok := resp.(*imap.DataResp)
	if !ok || data.Tag != "*" || len(data.Fields) == 0 {
		return nil
	}
	if name, ok := data.Fields[0].(string); !ok || !strings.EqualFold(name, "SORT") {
		return nil
	}
	for _, f := range data.Fields[1:] {
		if n, ok := f.(uint32); ok {
			s.uids = append(s.uids, n)
		}
	}
	return nil
}

// uidSort runs UID SORT with the given sort program and returns the ordered
// UID list. A NO/BAD status (e.g. a third-party server without SORT) comes
// back as an error so callers can fall back to local sorting.
func (c *Client) uidSort(cli *pooledConn, spec string) ([]uint32, error) {
	h := &sortCollector{}
	status, err := cli.Execute(&sortCmd{spec: spec}, h)
	if err != nil {
		return nil, fmt.Errorf("imap UID SORT: %w", err)
	}
	if status != nil {
		if err := status.Err(); err != nil {
			return nil, fmt.Errorf("imap UID SORT: %w", err)
		}
	}
	return h.uids, nil
}
