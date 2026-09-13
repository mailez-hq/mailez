// Charset handling for message text.
//
// Labels are resolved through the WHATWG label registry
// (golang.org/x/text/encoding/htmlindex) instead of a hand-written switch: it
// covers every alias real mail uses — gb2312/gbk/cp936/gb18030, big5/cp950,
// shift_jis/sjis/cp932, euc-jp, euc-kr/ks_c_5601-1987, iso-8859-*, windows-125x
// /cp125x, koi8-r/u, mac-roman, utf-16 — and keeps the aliases right. A switch
// silently passes through anything it does not name, which is how a whole body
// ends up as U+FFFD replacement characters.
package mail

import (
	"bytes"
	"io"
	"mime"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	xunicode "golang.org/x/text/encoding/unicode"
)

// headerWordDecoder decodes RFC 2047 encoded words in header values and MIME
// parameters, resolving their charsets through the same registry as the body
// parts. The stdlib decoder knows only UTF-8 and ISO-8859-1, which is why
// "=?GBK?B?...?=" attachment names reached the download list as the encoded
// word.
var headerWordDecoder = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		enc, err := htmlindex.Get(charset)
		if err != nil {
			return nil, err
		}
		return enc.NewDecoder().Reader(input), nil
	},
}

// decodeHeaderWords decodes RFC 2047 words in a header value or MIME
// parameter, keeping the raw value when a word cannot be decoded.
func decodeHeaderWords(s string) string {
	if !strings.Contains(s, "=?") {
		return s
	}
	decoded, err := headerWordDecoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}

// decodeCharset converts text in a declared charset to UTF-8.
//
// The order is the one mail clients use:
//  1. a byte-order mark, which is authoritative;
//  2. the declared label, resolved through the WHATWG registry;
//  3. sniffing — when the label is missing or provably inconsistent with the
//     bytes (us-ascii carrying 8-bit data, utf-8 with invalid sequences, an
//     unknown label).
//
// A label that decodes cleanly is never overridden: RFC 2046 makes it
// authoritative, so second-guessing it would corrupt correctly labelled
// Latin mail. Unknown labels and undecodable bytes are the cases where the
// sender is demonstrably wrong.
func decodeCharset(b []byte, charset string) []byte {
	if len(b) == 0 {
		return b
	}
	if out, ok := decodeBOM(b); ok {
		return out
	}
	label := strings.TrimSpace(charset)
	if label != "" && !labelProvablyWrong(label, b) {
		if enc, err := htmlindex.Get(label); err == nil {
			if out, clean := decodeClean(enc, b); clean {
				return out
			}
		}
	}
	return sniffCharset(b)
}

// labelProvablyWrong reports the declarations that cannot describe the bytes:
// us-ascii carrying 8-bit data (ASCII is a 7-bit charset) and utf-8 with
// invalid sequences. Those are repaired by sniffing.
//
// A label that merely *could* be wrong — iso-8859-1 over GBK bytes, say — is
// honoured: RFC 2046 makes the label authoritative, browsers and mail clients
// do the same, and second-guessing every western label would corrupt correctly
// labelled Latin mail.
func labelProvablyWrong(label string, b []byte) bool {
	switch strings.ToLower(label) {
	case "us-ascii", "ascii", "ansi_x3.4-1968", "iso-ir-6", "iso646-us":
		for _, c := range b {
			if c > 0x7F {
				return true
			}
		}
	case "utf-8", "utf8":
		return !utf8.Valid(b)
	}
	return false
}

// decodeBOM decodes a text part whose byte-order mark names its encoding. A
// BOM outranks the label: senders that emit one are telling the truth, and the
// label on such messages is often a stale default.
func decodeBOM(b []byte) ([]byte, bool) {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return b[3:], true
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		out, err := xunicode.UTF16(xunicode.LittleEndian, xunicode.IgnoreBOM).NewDecoder().Bytes(b[2:])
		if err != nil {
			return b, false
		}
		return out, true
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		out, err := xunicode.UTF16(xunicode.BigEndian, xunicode.IgnoreBOM).NewDecoder().Bytes(b[2:])
		if err != nil {
			return b, false
		}
		return out, true
	}
	return nil, false
}

// decodeClean decodes with enc and reports whether the result is usable:
// no decoder error and no replacement characters (the decoder's way of
// reporting bytes that are not valid in that charset).
func decodeClean(enc encoding.Encoding, b []byte) ([]byte, bool) {
	out, err := enc.NewDecoder().Bytes(b)
	if err != nil && len(out) == 0 {
		return nil, false
	}
	if bytes.ContainsRune(out, utf8.RuneError) {
		return nil, false
	}
	return out, true
}

// sniffCandidates are tried, in order of how likely they are to be the sender's
// real encoding for mail that reaches this product: Simplified Chinese first
// (the common case), then the other CJK charsets and the legacy western ones.
// All of them are in the WHATWG registry.
var sniffCandidates = []string{
	"utf-8",
	"gb18030",
	"big5",
	"shift_jis",
	"euc-kr",
	"euc-jp",
	"windows-1252",
	"windows-1251",
	"koi8-r",
}

// sniffCharset recovers text whose charset the sender omitted or mislabelled.
// Every candidate decodes the same bytes; the one whose output looks most like
// text in a script a mail can plausibly be written in wins. Bytes that no
// candidate explains are returned untouched — showing the raw bytes beats
// inventing characters.
func sniffCharset(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	best := b
	bestScore := 0.5 // below this, no candidate is a better story than the bytes
	for _, label := range sniffCandidates {
		enc, err := htmlindex.Get(label)
		if err != nil {
			continue
		}
		out, clean := decodeClean(enc, b)
		if !clean {
			continue
		}
		if score := scriptScore(out); score > bestScore {
			best, bestScore = out, score
		}
	}
	return best
}

// scriptScore reports how much of the decoded text looks like writing rather
// than a decoding accident: letters, digits, punctuation and the CJK/Kana/
// Hangul ranges count, a byte that decoded into a control character does not.
// Chinese text decoded as Shift_JIS, for instance, lands mostly in control and
// symbol ranges and scores low, so the candidates can be compared without
// language detection.
func scriptScore(b []byte) float64 {
	total, good := 0, 0
	for _, r := range string(b) {
		total++
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			good++
		case unicode.IsPrint(r) && !unicode.Is(unicode.Cs, r):
			// CJK, Kana, Hangul, Latin, Greek, Cyrillic — the ranges a decoded
			// text part actually uses. Symbols and control characters do not
			// count, which is what separates a real decoding from a wrong one.
			if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSpace(r) {
				good++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(good) / float64(total)
}
