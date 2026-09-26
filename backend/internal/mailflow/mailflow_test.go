package mailflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

func newMailflowDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mailflow.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDecorateAppendsSignatureForClientsThatDoNotEmbedOne(t *testing.T) {
	db := newMailflowDB(t)
	if err := db.Create(&models.Signature{
		UserEmail: "a@example.com", Name: "Work",
		BodyHTML: "<p>Ada</p>", BodyText: "Ada",
		DefaultForNew: true, DefaultForReply: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	text, html := Decorate(db, Options{
		OwnerEmail: "a@example.com", From: "a@example.com",
		Text: "Hi there", HTML: "<p>Hi there</p>",
	})
	if !strings.Contains(text, "Hi there\n\n-- \nAda") {
		t.Fatalf("plain-text signature missing: %q", text)
	}
	if !strings.Contains(html, `data-mailez-signature="server"`) || !strings.Contains(html, "<p>Ada</p>") {
		t.Fatalf("html signature missing: %q", html)
	}
	if !strings.HasPrefix(html, "<p>Hi there</p>") {
		t.Fatalf("signature must follow the body: %q", html)
	}
}

func TestDecorateSkipsEmbeddedSignature(t *testing.T) {
	db := newMailflowDB(t)
	if err := db.Create(&models.Signature{
		UserEmail: "a@example.com", Name: "Work", BodyHTML: "<p>Ada</p>", BodyText: "Ada", DefaultForNew: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	body := `<p>Hi</p><div data-mailez-signature="3"><p>--</p><p>Ada</p></div>`
	text, html := Decorate(db, Options{
		OwnerEmail: "a@example.com", From: "a@example.com",
		Text: "Hi\n\n-- \nAda", HTML: body, SignatureApplied: true,
	})
	if text != "Hi\n\n-- \nAda" || html != body {
		t.Fatalf("an embedded signature must be left alone:\n%q\n%q", text, html)
	}
	text2, html2 := Decorate(db, Options{
		OwnerEmail: "a@example.com", From: "a@example.com",
		Text: "Hi", HTML: body,
	})
	if text2 != "Hi" || html2 != body {
		t.Fatalf("marker detection failed: %q %q", text2, html2)
	}
}

func TestDecorateUsesIdentityScopeAndReplyDefault(t *testing.T) {
	db := newMailflowDB(t)
	seed := []models.Signature{
		{UserEmail: "a@example.com", Name: "generic", BodyHTML: "<p>generic</p>", BodyText: "generic", DefaultForNew: true},
		{UserEmail: "a@example.com", Name: "reply", BodyHTML: "<p>reply</p>", BodyText: "reply", DefaultForReply: true},
		{UserEmail: "a@example.com", IdentityEmail: "sales@example.com", Name: "sales", BodyHTML: "<p>sales</p>", BodyText: "sales", DefaultForNew: true},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	_, html := Decorate(db, Options{OwnerEmail: "a@example.com", From: "sales@example.com", Text: "x", HTML: "<p>x</p>"})
	if !strings.Contains(html, "<p>sales</p>") {
		t.Fatalf("identity signature not used: %q", html)
	}
	_, replyHTML := Decorate(db, Options{OwnerEmail: "a@example.com", From: "a@example.com", Text: "x", HTML: "<p>x</p>", Reply: true})
	if !strings.Contains(replyHTML, "<p>reply</p>") {
		t.Fatalf("reply default not used: %q", replyHTML)
	}
	text, html2 := Decorate(db, Options{OwnerEmail: "nobody@example.com", From: "nobody@example.com", Text: "x", HTML: "<p>x</p>"})
	if text != "x" || html2 != "<p>x</p>" {
		t.Fatalf("unexpected decoration: %q %q", text, html2)
	}
}

func TestDecorateLeavesEmptyPartsEmpty(t *testing.T) {
	db := newMailflowDB(t)
	if err := db.Create(&models.Signature{
		UserEmail: "a@example.com", Name: "Work", BodyHTML: "<p>Ada</p>", BodyText: "Ada", DefaultForNew: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	text, html := Decorate(db, Options{OwnerEmail: "a@example.com", From: "a@example.com", Text: "", HTML: "<p>Hi</p>"})
	if text != "" {
		t.Fatalf("empty text part should stay empty, got %q", text)
	}
	if !strings.Contains(html, "<p>Ada</p>") {
		t.Fatalf("html part not decorated: %q", html)
	}
}

func TestDecorateAppendsOrgFooterAfterSignature(t *testing.T) {
	db := newMailflowDB(t)
	if err := db.Create(&models.Signature{
		UserEmail: "a@example.com", Name: "Work", BodyHTML: "<p>Ada</p>", BodyText: "Ada", DefaultForNew: true,
	}).Error; err != nil {
		t.Fatalf("seed signature: %v", err)
	}
	footers := []models.OrgFooter{
		{Domain: "example.com", BodyHTML: "<p>Confidential — ACME</p>", BodyText: "Confidential — ACME", Enabled: true},
		{Domain: "disabled.example", BodyHTML: "<p>nope</p>", BodyText: "nope", Enabled: false},
	}
	for i := range footers {
		if err := db.Create(&footers[i]).Error; err != nil {
			t.Fatalf("seed footer: %v", err)
		}
	}

	text, html := Decorate(db, Options{OwnerEmail: "a@example.com", From: "a@example.com", Text: "Hi", HTML: "<p>Hi</p>"})
	if !strings.Contains(html, `data-mailez-footer="1"`) || !strings.Contains(html, "Confidential — ACME") {
		t.Fatalf("footer missing: %q", html)
	}
	if strings.Index(html, "data-mailez-signature") > strings.Index(html, "data-mailez-footer") {
		t.Fatalf("footer must follow the signature: %q", html)
	}
	if !strings.HasSuffix(text, "Confidential — ACME") {
		t.Fatalf("plain-text footer missing: %q", text)
	}
	if strings.Count(text, "Ada") != 1 {
		t.Fatalf("signature duplicated: %q", text)
	}

	_, other := Decorate(db, Options{OwnerEmail: "a@example.com", From: "a@disabled.example", Text: "Hi", HTML: "<p>Hi</p>"})
	if strings.Contains(other, "nope") || strings.Contains(other, "data-mailez-footer") {
		t.Fatalf("disabled footer applied: %q", other)
	}
	_, none := Decorate(db, Options{OwnerEmail: "a@example.com", From: "a@other.example", Text: "Hi", HTML: "<p>Hi</p>"})
	if strings.Contains(none, "data-mailez-footer") {
		t.Fatalf("footer applied outside its domain: %q", none)
	}
}

func TestAppendFooterPartsDoesNotDoubleApply(t *testing.T) {
	text, html := AppendFooterParts("Hi", "<p>Hi</p>", "ACME", "<p>ACME</p>")
	againText, againHTML := AppendOrgFooterPartsGuard(text, html)
	if strings.Count(againText, "ACME") != 1 || strings.Count(againHTML, "ACME") != 1 {
		t.Fatalf("footer stacked:\n%q\n%q", againText, againHTML)
	}
}

func AppendOrgFooterPartsGuard(text, html string) (string, string) {
	if strings.Contains(html, footerMarker) {
		return text, html
	}
	return AppendFooterParts(text, html, "ACME", "<p>ACME</p>")
}
