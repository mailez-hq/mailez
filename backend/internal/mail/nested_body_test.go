package mail

import (
	"strings"
	"testing"
)

// A body wrapped in multipart/mixed > multipart/alternative (what most
// mailers emit) must still yield the text and HTML parts. Regression: a
// single-level walk missed the nested alternative and the webmail rendered
// the message blank.
func TestExtractBodyNestedMultipart(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"To: rcpt@example.com\r\n" +
		"Subject: nested\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=outer\r\n" +
		"\r\n" +
		"--outer\r\n" +
		"Content-Type: multipart/alternative; boundary=inner\r\n" +
		"\r\n" +
		"--inner\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"plain body\r\n" +
		"--inner\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>html body</p>\r\n" +
		"--inner--\r\n" +
		"--outer--\r\n"
	text, html, attachments, inv, err := extractBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if text != "plain body" {
		t.Errorf("text = %q, want %q", text, "plain body")
	}
	if !strings.Contains(html, "html body") {
		t.Errorf("html = %q, want it to contain %q", html, "html body")
	}
	if len(attachments) != 0 || inv != nil {
		t.Errorf("unexpected attachments/invitation: %v %v", attachments, inv)
	}
}
