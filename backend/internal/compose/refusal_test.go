package compose

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"
)

// sendError makes the fake gateway fail the delivery with err, so the
// handler's error mapping can be exercised without a mail service.
type sendError struct {
	mail.Gateway
	err error
}

func (f *sendError) With(mail.Dial) mail.Gateway { return f }

func (f *sendError) Send(string, string, string, []string, []string, []string, string, string, string, []mail.Attachment, ...mail.Header) error {
	return f.err
}

func postJSON(t *testing.T, app interface {
	Test(*http.Request, ...int) (*http.Response, error)
}, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// The webmail picker refuses a 20 MB+ attachment, but the API is reachable
// directly and the gateway only bounds the whole request body — so the limit
// has to be enforced server-side too.
func TestSendRejectsOversizedAttachment(t *testing.T) {
	app, _ := newTestApp(t, &fakeGateway{}, func(a *core.App) { a.Cfg.MaxAttachmentBytes = 1024 })
	big := base64.StdEncoding.EncodeToString(make([]byte, 4096))
	status, body := postJSON(t, app, `{"to":["b@example.com"],"body":"hi","attachments":[{"filename":"big.bin","content_type":"application/octet-stream","data":"`+big+`"}]}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%s), want 422", status, body)
	}
	if !strings.Contains(body, "attachment_too_large") {
		t.Fatalf("body = %s, want attachment_too_large", body)
	}
}

// A typo in a recipient is a client error: it used to answer a bare 502
// "mail service error" that told the sender nothing.
func TestSendMapsRecipientRefusal(t *testing.T) {
	app, _ := newTestApp(t, &sendError{err: errors.New(`smtp rcpt not-an-email: 501 5.1.3 Bad recipient address syntax`)})
	status, body := postJSON(t, app, `{"to":["not-an-email"],"body":"hi"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%s), want 422", status, body)
	}
	if !strings.Contains(body, "recipient_rejected") {
		t.Fatalf("body = %s, want recipient_rejected", body)
	}
}

// An unreachable engine is temporary, so it answers 503 with a sentence
// rather than a bare 502 body the caller cannot act on.
func TestSendMapsEngineDown(t *testing.T) {
	app, _ := newTestApp(t, &sendError{err: fmt.Errorf("smtp dial: %w", syscall.ECONNREFUSED)})
	status, body := postJSON(t, app, `{"to":["b@example.com"],"body":"hi"}`)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s), want 503", status, body)
	}
	if !strings.Contains(body, "mail_service_unavailable") {
		t.Fatalf("body = %s, want mail_service_unavailable", body)
	}
}
