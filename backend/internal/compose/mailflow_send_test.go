package compose

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func newMailflowSendApp(t *testing.T) (*fiber.App, *fakeGateway, *gorm.DB) {
	t.Helper()
	var db *gorm.DB
	app, fake := newTestApp(t, &fakeGateway{}, func(a *core.App) { db = a.DB })
	return app, fake, db
}

func TestMailSendSignsAndFootersServerSide(t *testing.T) {
	app, fake, db := newMailflowSendApp(t)
	seed := []any{
		&models.Signature{
			UserEmail: "a@example.com", Name: "Work", BodyHTML: "<p>Ada</p>", BodyText: "Ada",
			DefaultForNew: true, DefaultForReply: true,
		},
		&models.OrgFooter{
			Domain: "example.com", BodyHTML: "<p>ACME Confidential</p>", BodyText: "ACME Confidential", Enabled: true,
		},
	}
	for _, row := range seed {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	body, _ := json.Marshal(map[string]any{
		"to": []string{"b@example.com"}, "subject": "hi",
		"body": "hello", "html": "<p>hello</p>",
	})
	resp, out := doJSON(t, app, http.MethodPost, "/api/v1/mail/send", string(body))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("send status = %d (%s)", resp.StatusCode, out)
	}
	if !strings.Contains(fake.sentText, "hello\n\n-- \nAda") {
		t.Fatalf("text signature missing: %q", fake.sentText)
	}
	if !strings.HasSuffix(fake.sentText, "ACME Confidential") {
		t.Fatalf("text footer missing: %q", fake.sentText)
	}
	if !strings.Contains(fake.sentHTML, `data-mailez-signature="server"`) {
		t.Fatalf("html signature missing: %q", fake.sentHTML)
	}
	if !strings.Contains(fake.sentHTML, `data-mailez-footer="1"`) {
		t.Fatalf("html footer missing: %q", fake.sentHTML)
	}
	if strings.Index(fake.sentHTML, "data-mailez-signature") > strings.Index(fake.sentHTML, "data-mailez-footer") {
		t.Fatalf("footer must follow the signature: %q", fake.sentHTML)
	}
}

func TestMailSendKeepsClientSignatureAndStillFooters(t *testing.T) {
	app, fake, db := newMailflowSendApp(t)
	if err := db.Create(&models.OrgFooter{
		Domain: "example.com", BodyHTML: "<p>ACME Confidential</p>", BodyText: "ACME Confidential", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("seed footer: %v", err)
	}
	embedded := `<p>hello</p><p><br></p><div data-mailez-signature="7"><p>--</p><p>Ada</p></div>`
	body, _ := json.Marshal(map[string]any{
		"to": []string{"b@example.com"}, "subject": "hi",
		"body": "hello\n\n-- \nAda", "html": embedded, "signature_applied": true,
	})
	resp, out := doJSON(t, app, http.MethodPost, "/api/v1/mail/send", string(body))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("send status = %d (%s)", resp.StatusCode, out)
	}
	if strings.Count(fake.sentHTML, "data-mailez-signature") != 1 {
		t.Fatalf("composer signature must not be duplicated: %q", fake.sentHTML)
	}
	if !strings.HasPrefix(fake.sentHTML, embedded) {
		t.Fatalf("client body was rewritten: %q", fake.sentHTML)
	}
	if !strings.Contains(fake.sentHTML, `data-mailez-footer="1"`) {
		t.Fatalf("footer missing for a composer send: %q", fake.sentHTML)
	}
}

func TestMailSendUsesReplySignatureForThreadedSends(t *testing.T) {
	app, fake, db := newMailflowSendApp(t)
	seed := []models.Signature{
		{UserEmail: "a@example.com", Name: "new", BodyHTML: "<p>NEW</p>", BodyText: "NEW", DefaultForNew: true},
		{UserEmail: "a@example.com", Name: "reply", BodyHTML: "<p>REPLY</p>", BodyText: "REPLY", DefaultForReply: true},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	body, _ := json.Marshal(map[string]any{
		"to": []string{"b@example.com"}, "subject": "Re: hi",
		"body": "hello", "html": "<p>hello</p>", "in_reply_to": "<abc@example.com>",
	})
	resp, out := doJSON(t, app, http.MethodPost, "/api/v1/mail/send", string(body))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("send status = %d (%s)", resp.StatusCode, out)
	}
	if !strings.Contains(fake.sentHTML, "REPLY") || strings.Contains(fake.sentHTML, "NEW") {
		t.Fatalf("wrong signature for a reply: %q", fake.sentHTML)
	}
}
