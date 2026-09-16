package mail

import (
	"fmt"
	"strconv"
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
		switch v := f.(type) {
		case uint32:
			s.uids = append(s.uids, v)
		case string:
			// go-imap v1 returns atoms as strings, and a server may fold a
			// run into "n:m".
			s.appendAtom(v)
		case imap.SeqSet:
			for _, seq := range v.Set {
				s.appendRange(seq.Start, seq.Stop)
			}
		}
	}
	return nil
}

const sortRangeCap = 100000

// appendAtom handles a bare UID or a "start:stop" run; anything else is
// ignored.
func (s *sortCollector) appendAtom(atom string) {
	atom = strings.TrimSpace(atom)
	if atom == "" {
		return
	}
	if start, stop, ok := strings.Cut(atom, ":"); ok {
		lo, errLo := strconv.ParseUint(strings.TrimSpace(start), 10, 32)
		hi, errHi := strconv.ParseUint(strings.TrimSpace(stop), 10, 32)
		if errLo != nil || errHi != nil {
			return
		}
		s.appendRange(uint32(lo), uint32(hi))
		return
	}
	if n, err := strconv.ParseUint(atom, 10, 32); err == nil {
		s.uids = append(s.uids, uint32(n))
	}
}

func (s *sortCollector) appendRange(start, stop uint32) {
	// go-imap represents "n:*" as Stop = 0. The upper bound is the mailbox's
	// highest UID, which a SORT response never carries.
	if stop == 0 {
		return
	}
	if stop < start {
		start, stop = stop, start
	}
	if uint64(stop-start)+1 > sortRangeCap {
		return
	}
	for n := start; n <= stop; n++ {
		s.uids = append(s.uids, n)
	}
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
