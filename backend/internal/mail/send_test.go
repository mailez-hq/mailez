package mail

import (
	"strings"
	"testing"
)

func TestBuildMessagePlain(t *testing.T) {
	msg := buildMessage("a@x.test", "b@x.test", "hi", "hello world", "")
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
	msg := buildMessage("a@x.test", "b@x.test", "hi", "plain body", "<p>html body</p>")
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
