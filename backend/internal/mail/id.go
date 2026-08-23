package mail

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/emersion/go-imap"
)

// Message ID helpers.
//
// The routable ID we expose for a message must be reversible back to the
// underlying IMAP UID without any local storage (this gateway is stateless
// across requests and nodes). We derive it from the RFC Message-ID header,
// which is guaranteed unique and immutable across mailboxes, and encode it
// URL-safe (base64url). Because Message-ID already carries a random token,
// the encoded form is unguessable (not enumerable), while decoding it lets us
// look the message up by searching HEADER Message-ID on the server.

// EncodeMessageID wraps the raw Message-ID (e.g. "<abc123@example.com>") into
// a URL-safe opaque id. A missing/empty Message-ID yields "." so callers can
// still build a degenerate (non-stable) route for that rare case.
func EncodeMessageID(msgID string) string {
	msgID = strings.TrimSpace(msgID)
	if msgID == "" {
		return "."
	}
	return base64.RawURLEncoding.EncodeToString([]byte(msgID))
}

// DecodeMessageID undoes EncodeMessageID. It returns an empty string (and no
// error) for the degenerate "." form.
func DecodeMessageID(id string) (string, error) {
	if id == "." || id == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UIDByMessageID searches a mailbox for the first message whose Message-ID
// header matches the given encoded id and returns its IMAP UID. This is the
// inverse of EncodeMessageID: it lets a stateless gateway resolve a routable
// id back into the UID used for all subsequent operations. It returns
// ErrNotFound when the message is absent from the mailbox.
func (c *Client) UIDByMessageID(email, token, folder, id string) (uint32, error) {
	msgID, err := DecodeMessageID(id)
	if err != nil {
		return 0, fmt.Errorf("decode message id: %w", err)
	}
	if msgID == "" {
		return 0, errors.New("message id unavailable")
	}

	cli, err := c.openIMAP(email, token)
	if err != nil {
		return 0, err
	}
	defer cli.Logout()

	if _, err := cli.Select(folder, true); err != nil {
		return 0, fmt.Errorf("imap select %q: %w", folder, err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Header["Message-ID"] = []string{msgID}
	uids, err := cli.Search(criteria)
	if err != nil {
		return 0, fmt.Errorf("imap search message-id: %w", err)
	}
	if len(uids) == 0 {
		return 0, errors.New("message not found")
	}
	return uids[0], nil
}
