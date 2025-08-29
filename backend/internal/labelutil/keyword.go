package labelutil

import (
	"fmt"
	"strings"
)

// MaxKeywordBytes mirrors the IMAP keyword atom length limit (64 octets).
// Display names whose encoded keyword exceeds this are rejected.
const MaxKeywordBytes = 64

// atomSafe reports whether r may appear literally inside an IMAP flag
// keyword (atom). Printable ASCII is allowed except the atom-specials
// ( ) { SP CTL % * " \ ] from RFC 9051.
func atomSafe(r rune) bool {
	if r < 0x21 || r > 0x7e {
		return false
	}
	switch r {
	case '(', ')', '{', '%', '*', '"', '\\', ']':
		return false
	}
	return true
}

// EncodeKeyword maps a human-readable label name to the ASCII keyword that is
// actually stored on the IMAP wire. Pure-ASCII names pass through unchanged
// (backward compatible with existing labels); every other character is
// encoded as =XX per UTF-8 byte, following the same approach Mozilla adopted
// for non-ASCII client tags. The result is a valid IMAP atom and is
// deterministic, so name→keyword always agrees between save, flag, rename,
// delete and search.
func EncodeKeyword(name string) string {
	var b strings.Builder
	for _, r := range name {
		if atomSafe(r) {
			b.WriteRune(r)
			continue
		}
		for _, c := range []byte(string(r)) {
			fmt.Fprintf(&b, "=%02X", c)
		}
	}
	return b.String()
}

// systemFlags maps every accepted spelling of an IMAP system flag (RFC 3501
// §2.3.2: case-insensitive; clients also send the bare display name) to its
// canonical wire form. Without this mapping an API caller posting flag:"seen"
// silently creates a custom *keyword* named "seen" that no UNSEEN count, flag
// filter or expunge ever matches.
var systemFlags = map[string]string{
	"seen":       "\\Seen",
	"\\seen":     "\\Seen",
	"flagged":    "\\Flagged",
	"\\flagged":  "\\Flagged",
	"answered":   "\\Answered",
	"\\answered": "\\Answered",
	"deleted":    "\\Deleted",
	"\\deleted":  "\\Deleted",
	"draft":      "\\Draft",
	"\\draft":    "\\Draft",
}

// CanonicalFlag resolves a user-supplied flag name to its canonical system
// flag when it names one (case-insensitive, leading backslash optional).
// Everything else — custom keywords, encoded label names — is returned
// unchanged for the label-keyword fallback to handle.
func CanonicalFlag(name string) string {
	if v, ok := systemFlags[strings.ToLower(name)]; ok {
		return v
	}
	return name
}
