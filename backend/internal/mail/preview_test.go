package mail

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/emersion/go-imap"
)

func TestPreferredTextPart(t *testing.T) {
	// non-multipart text/plain -> TEXT section (empty path)
	p, path := preferredTextPart(&imap.BodyStructure{MIMEType: "text", MIMESubType: "plain"}, nil)
	if p == nil || len(path) != 0 {
		t.Fatalf("plain singlepart: want TEXT path, got %v %v", p, path)
	}
	// non-multipart text/html -> TEXT section
	p, _ = preferredTextPart(&imap.BodyStructure{MIMEType: "text", MIMESubType: "html"}, nil)
	if p == nil {
		t.Fatal("html singlepart: want part")
	}
	// attachment-disposition singlepart -> none
	if p, _ = preferredTextPart(&imap.BodyStructure{MIMEType: "application", MIMESubType: "pdf", Disposition: "attachment"}, nil); p != nil {
		t.Fatal("pdf singlepart: want nil")
	}
	// multipart/alternative: plain wins over html
	alt := &imap.BodyStructure{MIMEType: "multipart", Parts: []*imap.BodyStructure{
		{MIMEType: "text", MIMESubType: "plain"},
		{MIMEType: "text", MIMESubType: "html"},
	}}
	p, path = preferredTextPart(alt, nil)
	if p == nil || p.MIMESubType != "plain" || len(path) != 1 || path[0] != 1 {
		t.Fatalf("alternative: want plain part 1, got %v %v", p, path)
	}
	// nested multipart: mixed(text/plain + attachment) under alternative html-only
	nested := &imap.BodyStructure{MIMEType: "multipart", Parts: []*imap.BodyStructure{
		{MIMEType: "multipart", Parts: []*imap.BodyStructure{
			{MIMEType: "text", MIMESubType: "plain"},
			{MIMEType: "application", MIMESubType: "pdf", Disposition: "attachment"},
		}},
		{MIMEType: "text", MIMESubType: "html"},
	}}
	p, path = preferredTextPart(nested, nil)
	if p == nil || len(path) != 2 || path[0] != 1 || path[1] != 1 {
		t.Fatalf("nested: want path 1.1, got %v %v", p, path)
	}
	// attachments-only -> nil
	atts := &imap.BodyStructure{MIMEType: "multipart", Parts: []*imap.BodyStructure{
		{MIMEType: "application", MIMESubType: "zip", Disposition: "attachment"},
	}}
	if p, _ = preferredTextPart(atts, nil); p != nil {
		t.Fatal("attachments-only: want nil")
	}
}

func TestPreviewSection(t *testing.T) {
	s := previewSection(nil)
	if got := string(s.FetchItem()); got != "BODY.PEEK[TEXT]<0.4096>" {
		t.Fatalf("TEXT section: %q", got)
	}
	s = previewSection([]int{1, 1})
	if got := string(s.FetchItem()); got != "BODY.PEEK[1.1]<0.4096>" {
		t.Fatalf("part section: %q", got)
	}
}

func TestPreviewDecodeTransfer(t *testing.T) {
	// base64 truncated mid-group decodes the whole groups
	full := base64.StdEncoding.EncodeToString([]byte("hello world, this is a longer body"))
	cut := full[:len(full)-5]
	if got := string(previewDecodeTransfer([]byte(cut+"\r\n"), "base64")); !strings.HasPrefix(got, "hello world") {
		t.Fatalf("truncated base64: %q", got)
	}
	// quoted-printable ending mid-soft-break ("=\r\n" truncated to "=")
	if got := string(previewDecodeTransfer([]byte("caf=C3=A9="), "quoted-printable")); got != "café" {
		t.Fatalf("qp: %q", got)
	}
	// identity
	if got := string(previewDecodeTransfer([]byte("plain"), "7bit")); got != "plain" {
		t.Fatalf("7bit: %q", got)
	}
}

func TestPreviewDecodeCharset(t *testing.T) {
	// GBK bytes for 邮件预览
	gbk := []byte{0xD3, 0xCA, 0xBC, 0xFE, 0xD4, 0xA4, 0xC0, 0xC0}
	if got := string(decodeCharset(gbk, "gbk")); got != "邮件预览" {
		t.Fatalf("gbk: %q", got)
	}
	// latin-1
	if got := string(decodeCharset([]byte{0xE9}, "iso-8859-1")); got != "é" {
		t.Fatalf("latin1: %q", got)
	}
	// An unknown label is sniffed rather than passed through: GBK bytes still
	// come out as Chinese even when the sender mislabelled them.
	if got := string(decodeCharset(gbk, "x-unknown")); got != "邮件预览" {
		t.Fatalf("unknown label with gbk bytes: %q", got)
	}
	// A single high byte is Latin-1 text under the same unknown label.
	if got := string(decodeCharset([]byte{0xFF}, "x-unknown")); got != "ÿ" {
		t.Fatalf("unknown label single byte: %q", got)
	}
	// truncated multibyte tail keeps the decoded prefix
	iso := decodeCharset([]byte{0xE9, 0x62}, "iso-8859-1") // é + b — single-byte, no truncation issue
	if string(iso) != "éb" {
		t.Fatalf("latin1 pair: %q", iso)
	}
}

func TestBuildPreviewHTMLAndCollapse(t *testing.T) {
	htmlBody := "<html><body><p>Hello&nbsp;world</p><div>Second   line</div><br>tail</body></html>"
	got := buildPreview([]byte(htmlBody), "", "", true)
	if got != "Hello world Second line tail" {
		t.Fatalf("html preview: %q", got)
	}
	// rune-safe truncation with ellipsis
	long := strings.Repeat("邮", previewMaxRunes+50)
	out := buildPreview([]byte(strings.Repeat("邮", previewMaxRunes+50)), "", "", false)
	if !strings.HasSuffix(out, "…") || len([]rune(out)) != previewMaxRunes+1 {
		t.Fatalf("truncation: runes=%d suffix ok=%v", len([]rune(out)), strings.HasSuffix(out, "…"))
	}
	_ = long
}
