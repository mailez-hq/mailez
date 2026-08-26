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
// for non-ASCII Thunderbird tags. The result is a valid IMAP atom and is
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
