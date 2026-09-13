package mail

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/text/encoding/htmlindex"
)

// encode re-encodes a UTF-8 string the way a sender in that charset would.
func encode(t *testing.T, label, s string) []byte {
	t.Helper()
	enc, err := htmlindex.Get(label)
	if err != nil {
		t.Fatalf("htmlindex %s: %v", label, err)
	}
	out, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode %s: %v", label, err)
	}
	return out
}

// The charsets real mail arrives in, with a phrase that exercises non-ASCII in
// each script. The hand-written switch this replaced knew GBK, Big5 and a few
// western pages; everything else (Japanese, Korean, Cyrillic, UTF-16 parts)
// passed through as U+FFFD.
func TestDecodeCharsetMatrix(t *testing.T) {
	cases := []struct {
		label string
		text  string
	}{
		{"gb2312", "邮件预览"}, // the alias 126.com uses
		{"gbk", "邮件预览"},    // the one we shipped for
		{"gb18030", "邮件预览"},
		{"big5", "郵件預覽"},           // Traditional Chinese
		{"shift_jis", "メールのプレビュー"}, // Japanese
		{"euc-jp", "メールのプレビュー"},
		{"euc-kr", "메일 미리보기"}, // Korean
		{"windows-1251", "Почта"},
		{"koi8-r", "Почта"},
		{"iso-8859-1", "café"},
		{"windows-1252", "“curly”"},
		{"iso-8859-7", "Ελληνικά"}, // Greek
		{"windows-1256", "بريد"},   // Arabic
		{"windows-1255", "דואר"},   // Hebrew
		{"tis-620", "อีเมล"},       // Thai
		{"windows-1258", "thư"},
		{"utf-16le", "邮件"}, // with BOM, as Go encodes it
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			raw := encode(t, tc.label, tc.text)
			got := string(decodeCharset(raw, tc.label))
			if got != tc.text {
				t.Fatalf("%s: %q -> %q, want %q", tc.label, raw, got, tc.text)
			}
			// With no label at all the bytes must at least never turn into
			// replacement characters (which is what the JSON encoder makes of
			// invalid UTF-8). Which CJK charset wins is decided below.
			if sniffed := string(decodeCharset(raw, "")); strings.ContainsRune(sniffed, '\uFFFD') {
				t.Fatalf("%s: unlabelled sniff produced replacement characters: %q", tc.label, sniffed)
			}
		})
	}
}

// Unlabelled 8-bit text is a guess: the sniffer prefers the Chinese encodings
// (this product's traffic), so Big5/Shift_JIS/EUC-KR bytes without a label come
// out as plausible-but-wrong Chinese rather than as mojibake. Telling those
// apart needs statistical detection, which is deliberately not a dependency
// here — the reported failures were all *labelled* mail.
func TestDecodeCharsetUnlabelledPrefersChinese(t *testing.T) {
	if got := string(decodeCharset(encode(t, "gbk", "接收邮件呢"), "")); got != "接收邮件呢" {
		t.Fatalf("unlabelled gbk = %q", got)
	}
	if got := string(decodeCharset(encode(t, "big5", "郵件預覽"), "")); got == "郵件預覽" {
		t.Fatal("unlabelled big5 decoded exactly; if detection improved, update this expectation")
	}
}

// Senders mislabel more often than they invent charsets: us-ascii carrying
// 8-bit bytes and utf-8 carrying invalid sequences are both provably wrong, so
// the bytes win.
func TestDecodeCharsetRepairsWrongLabel(t *testing.T) {
	gbk := encode(t, "gbk", "接收邮件呢？有问题吗？")
	for _, label := range []string{"us-ascii", "ascii", "utf-8", "x-unknown", ""} {
		if got := string(decodeCharset(gbk, label)); got != "接收邮件呢？有问题吗？" {
			t.Fatalf("label %q: %q", label, got)
		}
	}
}

// A correctly labelled part is never second-guessed. RFC 2046 makes the label
// authoritative, and WHATWG's registry keeps the browser-compatible aliases:
// iso-8859-1 *is* windows-1252, which is why 0x93/0x94 come out as curly
// quotes rather than C1 control characters.
func TestDecodeCharsetHonoursGoodLabel(t *testing.T) {
	raw := []byte{0x93, 0x94} // windows-1252 curly quotes
	if got := string(decodeCharset(raw, "windows-1252")); got != "“”" {
		t.Fatalf("windows-1252 = %q", got)
	}
	// The latin-1 alias resolves to the same encoding (WHATWG), matching
	// browsers and mail clients.
	if got := string(decodeCharset(raw, "iso-8859-1")); got != "“”" {
		t.Fatalf("iso-8859-1 = %q", got)
	}
	// A Cyrillic label on Cyrillic bytes stays as declared.
	ru := encode(t, "windows-1251", "Почта")
	if got := string(decodeCharset(ru, "windows-1251")); got != "Почта" {
		t.Fatalf("windows-1251 = %q", got)
	}
}

// Attachment names arrive as RFC 2047 words far more often than as RFC 2231.
func TestExtractBodyDecodesAttachmentFilename(t *testing.T) {
	// "=?GBK?B?...?=" for 报告.pdf, which Go's ParseMediaType leaves encoded.
	gbkName := "=?GBK?B?" + base64.StdEncoding.EncodeToString(encode(t, "gbk", "报告.pdf")) + "?="
	raw := "Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nbody\r\n" +
		"--b1\r\nContent-Type: application/pdf; name=\"" + gbkName + "\"\r\n" +
		"Content-Disposition: attachment; filename=\"" + gbkName + "\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\nAAECAwQ=\r\n" +
		"--b1--\r\n"

	_, _, atts, _, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 || atts[0].Filename != "报告.pdf" {
		t.Fatalf("attachments = %+v", atts)
	}
}

// RFC 2231 names keep working (they were already decoded by ParseMediaType).
func TestExtractBodyKeepsRFC2231Filename(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\nbody\r\n" +
		"--b1\r\nContent-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename*=utf-8''%E6%8A%A5%E5%91%8A.pdf\r\n\r\nAAEC\r\n" +
		"--b1--\r\n"
	_, _, atts, _, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 || atts[0].Filename != "报告.pdf" {
		t.Fatalf("attachments = %+v", atts)
	}
}

// The same decoder backs the header-word path used for those names.
func TestDecodeHeaderWords(t *testing.T) {
	word := "=?Big5?B?" + base64.StdEncoding.EncodeToString(encode(t, "big5", "附件")) + "?="
	if got := decodeHeaderWords(word); got != "附件" {
		t.Fatalf("big5 word = %q", got)
	}
	if got := decodeHeaderWords("plain.txt"); got != "plain.txt" {
		t.Fatalf("plain = %q", got)
	}
	if got := decodeHeaderWords("=?X-NOPE?B?aGk=?="); got != "=?X-NOPE?B?aGk=?=" {
		t.Fatalf("undecodable word = %q", got)
	}
}

// A body encoded with a charset the registry knows but Go's encoding package
// cannot decode must not lose the payload: the raw bytes stay as they are.
func TestDecodeCharsetKeepsUndecodable(t *testing.T) {
	// Control bytes no text charset explains: they pass through untouched
	// rather than being dressed up as characters.
	raw := []byte{0x00, 0x01, 0x02}
	if got := decodeCharset(raw, "x-imaginary"); string(got) != string(raw) {
		t.Fatalf("control bytes = %v", got)
	}
}
