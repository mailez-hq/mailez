package mail

import (
	"strings"
	"testing"
)

func TestBuildMessagePlain(t *testing.T) {
	msg := BuildMessage("a@x.test", []string{"b@x.test"}, nil, "hi", "hello world", "", nil)
	for _, want := range []string{
		"From: a@x.test\r\n",
		"To: b@x.test\r\n",
		"Subject: hi\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"hello world",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("plain message missing %q", want)
		}
	}
	if strings.Contains(msg, "multipart/alternative") {
		t.Error("plain message must not be multipart")
	}
}

func TestBuildMessageMultipartAlternative(t *testing.T) {
	msg := BuildMessage("a@x.test", []string{"b@x.test"}, nil, "hi", "plain body", "<p>html body</p>", nil)
	body := msg[strings.Index(msg, "\r\n\r\n")+4:]
	if !strings.Contains(msg, "Content-Type: multipart/alternative; boundary=") {
		t.Errorf("missing multipart/alternative header in:\n%s", msg)
	}
	plainPart := strings.Index(body, "Content-Type: text/plain; charset=UTF-8\r\n")
	htmlPart := strings.Index(body, "Content-Type: text/html; charset=UTF-8\r\n")
	if plainPart < 0 || htmlPart < 0 || htmlPart <= plainPart {
		t.Errorf("expected text/plain part before text/html part in:\n%s", body)
	}
	if !strings.Contains(body[plainPart:htmlPart], "plain body") {
		t.Errorf("missing plain text content in:\n%s", body)
	}
	if !strings.Contains(body[htmlPart:], "<p>html body</p>") {
		t.Errorf("missing html content in:\n%s", body)
	}
	if !strings.HasSuffix(body, "--\r\n") {
		t.Errorf("multipart boundary not closed:\n%s", body)
	}
}

func TestBuildMessageCustomFrom(t *testing.T) {
	msg := BuildMessage("team@x.test", []string{"b@x.test"}, nil, "hi", "hello", "", nil)
	if !strings.Contains(msg, "From: team@x.test\r\n") {
		t.Errorf("message must carry the selected From header:\n%s", msg)
	}
}

// TestDraftBccHeader pins the Drafts-copy behaviour: blind recipients are
// preserved as a Bcc header on the stored draft, and no header appears when
// there are none (transmitted messages never carry Bcc — see the injection
// test above).
func TestDraftBccHeader(t *testing.T) {
	h := draftBccHeader([]string{"x@x.test", "y@x.test"})
	if len(h) != 1 || h[0].Key != "Bcc" || h[0].Value != "x@x.test, y@x.test" {
		t.Fatalf("draftBccHeader = %+v", h)
	}
	if got := draftBccHeader(nil); got != nil {
		t.Fatalf("draftBccHeader(nil) = %+v, want nil", got)
	}
	msg := BuildMessage("a@x.test", []string{"b@x.test"}, nil, "hi", "body", "", nil, draftBccHeader([]string{"h@x.test"})...)
	if !strings.Contains(msg, "Bcc: h@x.test\r\n") {
		t.Errorf("draft missing Bcc header:\n%s", msg)
	}
}

func TestBuildMessageAttachmentsAndRecipients(t *testing.T) {
	msg := BuildMessage(
		"a@x.test",
		[]string{"b@x.test"},
		[]string{"c@x.test"},
		"hi",
		"plain body",
		"<p>html body</p>",
		[]Attachment{
			{Filename: "报告.txt", ContentType: "text/plain", Data: "5L2g5aW9"},
			{Filename: "pic.png", ContentType: "image/png", Data: "aGVsbG8="},
		},
	)
	for _, want := range []string{
		"To: b@x.test\r\n",
		"Cc: c@x.test\r\n",
		"Date: ",
		"Message-ID: <",
		"Content-Type: multipart/mixed; boundary=",
		"Content-Disposition: attachment; filename*=UTF-8''%E6%8A%A5%E5%91%8A.txt",
		"Content-Disposition: attachment; filename=\"pic.png\"",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q in:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Bcc:") {
		t.Error("bcc must never appear in message headers")
	}
}

func TestBuildMessageHeaderInjection(t *testing.T) {
	injected := "hi\r\nBcc: victim@evil.test\r\n\r\ninjected body"
	msg := BuildMessage(
		"a@x.test",
		[]string{"b@x.test\r\nBcc: victim@evil.test"},
		[]string{"c@x.test\nBcc: victim2@evil.test"},
		injected,
		"hello",
		"",
		[]Attachment{{Filename: "f.txt", ContentType: "text/plain\r\nBcc: victim3@evil.test", Data: "aGk="}},
	)
	if strings.Contains(msg, "\r\nBcc") || strings.Contains(msg, "\nBcc") {
		t.Errorf("header splitting survived (a Bcc header line can be created):\n%s", msg)
	}
	if strings.Contains(msg, "injected body") {
		t.Errorf("injected body part survived:\n%s", msg)
	}
	if !strings.Contains(msg, "Subject: hi") {
		t.Errorf("subject must keep its first line:\n%s", msg)
	}
}
