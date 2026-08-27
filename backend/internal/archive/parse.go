// Package archive implements compliance email archiving: the mail engine
// forwards every inbound/outbound message to the stack ingest endpoint, the
// backend stores it with a retention policy, and admins search, review and
// export the store (Coremail-style 归档/检索/审查/稽核).
package archive

import (
	"mime"
	"net/mail"
	"strings"
	"time"
)

// parsedHeaders are the metadata extracted from a raw RFC 5322 message.
type parsedHeaders struct {
	MessageID string
	From      string
	To        string
	Cc        string
	Subject   string
	Date      time.Time
}

// parseMessage extracts header metadata from raw message bytes. Parsing
// failures never reject the ingest: envelope data is authoritative for
// search, and unparsed headers simply stay empty.
func parseMessage(raw []byte) parsedHeaders {
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		return parsedHeaders{}
	}
	h := msg.Header
	out := parsedHeaders{
		MessageID: strings.Trim(strings.TrimSpace(h.Get("Message-ID")), "<>"),
		From:      strings.TrimSpace(h.Get("From")),
		To:        strings.TrimSpace(h.Get("To")),
		Cc:        strings.TrimSpace(h.Get("Cc")),
		Subject:   decodeHeader(strings.TrimSpace(h.Get("Subject"))),
	}
	if d, err := h.Date(); err == nil {
		out.Date = d
	}
	return out
}

// decodeHeader decodes RFC 2047 encoded-words ("=?utf-8?B?...?=") so search
// matches the human-readable subject.
func decodeHeader(v string) string {
	if !strings.Contains(v, "=?") {
		return v
	}
	dec := new(mime.WordDecoder)
	if s, err := dec.DecodeHeader(v); err == nil {
		return s
	}
	return v
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}
