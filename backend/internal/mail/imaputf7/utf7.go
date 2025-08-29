// Package imaputf7 decodes RFC 3501 §5.1.3 modified UTF-7 mailbox names.
//
// The mail engine always emits mailbox names on the wire in modified UTF-7
// (e.g. "工作" arrives as "&XfJT0ZAB-"); this decoder turns them back into
// UTF-8 for API responses. Inbound names are sent as raw UTF-8, which the
// engine's decoder accepts, so no encoder is needed here.
//
// Decoder logic adapted from mailezine's internal/utf7 (emersion/go-imap
// style implementation).
package imaputf7

import (
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	min = 0x20 // Minimum self-representing UTF-7 value
	max = 0x7E // Maximum self-representing UTF-7 value
)

var (
	b64Dec     = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+,")
	errInvalid = errors.New("imaputf7: invalid modified UTF-7")
)

// FolderName decodes a mailbox name from the wire. Names that fail to decode
// (or are already plain UTF-8) are returned unchanged so a quirk on a foreign
// server can never blank out a folder list.
func FolderName(src string) string {
	out, err := Decode(src)
	if err != nil {
		return src
	}
	return out
}

// Decode decodes a string encoded with modified UTF-7. Raw UTF-8 input is
// accepted.
func Decode(src string) (string, error) {
	if !utf8.ValidString(src) {
		return "", errors.New("invalid UTF-8")
	}

	var sb strings.Builder
	sb.Grow(len(src))

	ascii := true
	for i := 0; i < len(src); i++ {
		ch := src[i]

		if ch < min || (ch > max && ch < utf8.RuneSelf) {
			// Illegal code point in ASCII mode. Note, UTF-8 codepoints are
			// always allowed.
			return "", errInvalid
		}

		if ch != '&' {
			sb.WriteByte(ch)
			ascii = true
			continue
		}

		// Find the end of the Base64 or "&-" segment
		start := i + 1
		for i++; i < len(src) && src[i] != '-'; i++ {
			if src[i] == '\r' || src[i] == '\n' { // base64 package ignores CR and LF
				return "", errInvalid
			}
		}

		if i == len(src) { // Implicit shift ("&...")
			return "", errInvalid
		}

		if i == start { // Escape sequence "&-"
			sb.WriteByte('&')
			ascii = true
		} else { // Control or non-ASCII code points in base64
			if !ascii { // Null shift ("&...-&...-")
				return "", errInvalid
			}

			b := decodeSegment(src[start:i])
			if len(b) == 0 { // Bad encoding
				return "", errInvalid
			}
			sb.Write(b)

			ascii = false
		}
	}

	return sb.String(), nil
}

// decodeSegment extracts UTF-16-BE bytes from base64 data and converts them
// to UTF-8. A nil slice is returned if the encoding is invalid.
func decodeSegment(seg string) []byte {
	b64 := []byte(seg)
	if n := len(b64); n == 0 || b64[n-1] == '=' {
		return nil
	}
	var padded []byte
	if n := len(b64); n&3 == 0 {
		padded = b64
	} else {
		padded = make([]byte, n, n+2)
		copy(padded, b64)
		for len(padded)&3 != 0 {
			padded = append(padded, '=')
		}
	}

	buf := make([]byte, b64Dec.DecodedLen(len(padded)))
	n, err := b64Dec.Decode(buf, padded)
	if err != nil || n&1 == 1 {
		return nil
	}
	b := buf[:n]

	out := make([]byte, 0, n)
	for i := 0; i < n; i += 2 {
		r := rune(b[i])<<8 | rune(b[i+1])
		if utf16.IsSurrogate(r) {
			if i += 2; i == n {
				return nil
			}
			r2 := rune(b[i])<<8 | rune(b[i+1])
			if r = utf16.DecodeRune(r, r2); r == utf8.RuneError {
				return nil
			}
		} else if min <= r && r <= max {
			return nil
		}
		out = utf8.AppendRune(out, r)
	}
	return out
}
