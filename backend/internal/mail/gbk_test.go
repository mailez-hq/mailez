package mail

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// gbkBase64 is the transfer encoding 126.com/163.com actually use: base64 over
// GBK bytes.
func gbkBase64(t *testing.T, s string) string {
	t.Helper()
	enc, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(enc)
}

// The reported message: multipart/alternative with a GBK text part and a GBK
// HTML part. Forwarding those bytes raw produced a body of U+FFFD.
func TestExtractBodyDecodesGBKPart(t *testing.T) {
	want := "这是测试邮件，回复一下。\n\n在 2026-09-14 05:08:33，admin@mailez.cn 写道："
	raw := "Content-Type: multipart/alternative; boundary=\"b1\"\r\n" +
		"MIME-Version: 1.0\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain; charset=GBK\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		gbkBase64(t, want) + "\r\n" +
		"--b1\r\nContent-Type: text/html; charset=GBK\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		gbkBase64(t, "<p>"+want+"</p>") + "\r\n" +
		"--b1--\r\n"

	text, html, _, _, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	if !strings.Contains(html, "这是测试邮件") {
		t.Fatalf("html = %q", html)
	}
	if strings.ContainsRune(text, '\uFFFD') {
		t.Fatal("body still carries replacement characters")
	}
}

// A text part with an unknown charset must pass through byte-for-byte rather
// than be mangled by a wrong conversion.
func TestExtractBodyKeepsUnknownCharset(t *testing.T) {
	raw := "Content-Type: text/plain; charset=x-nonsense\r\n\r\nhello\r\n"
	text, _, _, _, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(text) != "hello" {
		t.Fatalf("text = %q", text)
	}
}

// A binary attachment must never be charset-converted, even when the part
// declares one: the bytes go back to the client exactly as sent.
func TestExtractBodyKeepsAttachmentBytes(t *testing.T) {
	blob := []byte{0x00, 0xFF, 0x10, 0x80}
	raw := "Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nbody\r\n" +
		"--b1\r\nContent-Type: application/octet-stream; charset=GBK\r\n" +
		"Content-Disposition: attachment; filename=\"blob.bin\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString(blob) + "\r\n" +
		"--b1--\r\n"

	_, _, atts, _, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("attachments = %d", len(atts))
	}
	got, err := base64.StdEncoding.DecodeString(atts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(blob) {
		t.Fatalf("attachment bytes changed: %v -> %v", blob, got)
	}
}
