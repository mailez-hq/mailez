package mail

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// SearchQuery is the structured form of an advanced search expression such as
// `from:amy subject:"weekly report" has:attachment before:2026-01-01`, or the
// output of the AI query interpreter.
type SearchQuery struct {
	Text          []string
	From          []string
	To            []string
	Subject       []string
	HasAttachment bool
	Before        *time.Time
	After         *time.Time
}

var searchTokenRe = regexp.MustCompile(`(?i)([a-z]+):"([^"]*)"|([a-z]+):(\S+)|(\S+)`)

func parseSearchQuery(q string) SearchQuery {
	var out SearchQuery
	for _, m := range searchTokenRe.FindAllStringSubmatch(q, -1) {
		var key, val string
		switch {
		case m[1] != "":
			key, val = m[1], m[2]
		case m[3] != "":
			key, val = m[3], m[4]
		case m[5] != "":
			out.Text = append(out.Text, m[5])
			continue
		}
		switch strings.ToLower(key) {
		case "from":
			out.From = append(out.From, val)
		case "to":
			out.To = append(out.To, val)
		case "subject":
			out.Subject = append(out.Subject, val)
		case "has":
			if strings.EqualFold(val, "attachment") {
				out.HasAttachment = true
			}
		case "before":
			if t, err := time.Parse("2006-01-02", val); err == nil {
				out.Before = &t
			}
		case "after":
			if t, err := time.Parse("2006-01-02", val); err == nil {
				out.After = &t
			}
		}
	}
	return out
}

// SearchMessages parses an advanced query expression and runs it.
func (c *Client) SearchMessages(email, token, folder, query string) ([]Message, error) {
	return c.SearchMessagesSpec(email, token, folder, parseSearchQuery(query))
}

// SearchMessagesSpec returns messages matching a structured query (subject,
// from, to, body, dates, attachment filter), newest first.
func (c *Client) SearchMessagesSpec(email, token, folder string, sel SearchQuery) ([]Message, error) {
	cli, err := c.openIMAP(email, token)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, true); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", folder, err)
	}
	criteria := imap.NewSearchCriteria()
	criteria.Text = sel.Text
	criteria.Header["From"] = sel.From
	criteria.Header["To"] = sel.To
	criteria.Header["Subject"] = sel.Subject
	if sel.Before != nil {
		criteria.Before = *sel.Before
	}
	if sel.After != nil {
		criteria.Since = *sel.After
	}
	uids, err := cli.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}
	if sel.HasAttachment {
		if uids, err = c.filterHasAttachment(cli, uids); err != nil {
			return nil, err
		}
	}
	if len(uids) == 0 {
		return []Message{}, nil
	}
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure}, messages)
	}()

	var out []Message
	for msg := range messages {
		out = append(out, envelopeToMessage(msg))
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap uidfetch: %w", err)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// filterHasAttachment keeps only UIDs whose body structure carries an
// attachment (standard IMAP has no direct "has attachment" search key).
func (c *Client) filterHasAttachment(cli *client.Client, uids []uint32) ([]uint32, error) {
	if len(uids) == 0 {
		return uids, nil
	}
	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seqset, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure}, messages)
	}()
	var out []uint32
	for msg := range messages {
		if hasAttachments(msg.BodyStructure) {
			out = append(out, msg.Uid)
		}
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap attachment filter: %w", err)
	}
	return out, nil
}
