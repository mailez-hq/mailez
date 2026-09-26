package models

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newSignatureTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "signatures.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSignatureHTMLRoundTrip(t *testing.T) {
	text := "Ada Lovelace\nCTO, Example\n\nada@example.com"
	htmlBody := PlainTextToSignatureHTML(text)
	if !strings.Contains(htmlBody, "<p>ada@example.com</p>") {
		t.Fatalf("plain text not rendered as paragraphs: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "<p><br></p>") {
		t.Fatalf("blank line not preserved: %s", htmlBody)
	}
	if got := SignatureHTMLToText(htmlBody); got != text {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, text)
	}
}

func TestSignatureHTMLToTextEscapesAndStripsTags(t *testing.T) {
	in := `<p>Ada &amp; Co</p><p><br></p><p><strong>Bold</strong> <a href="https://x.example">x</a></p>`
	want := "Ada & Co\n\nBold x"
	if got := SignatureHTMLToText(in); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBackfillSignatures(t *testing.T) {
	db := newSignatureTestDB(t)
	users := []User{
		{Email: "a@example.com", Localpart: "a", DomainName: "example.com", Enabled: true, Signature: "Ada\nada@example.com"},
		{Email: "b@example.com", Localpart: "b", DomainName: "example.com", Enabled: true, Signature: "   "},
		{Email: "c@example.com", Localpart: "c", DomainName: "example.com", Enabled: true},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	if err := backfillSignatures(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var rows []Signature
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one backfilled signature, got %d", len(rows))
	}
	got := rows[0]
	if got.UserEmail != "a@example.com" || !got.DefaultForNew || !got.DefaultForReply {
		t.Fatalf("unexpected backfilled row: %+v", got)
	}
	if got.BodyText != "Ada\nada@example.com" || !strings.Contains(got.BodyHTML, "<p>Ada</p>") {
		t.Fatalf("unexpected backfilled body: %+v", got)
	}
	if err := backfillSignatures(db); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var count int64
	db.Model(&Signature{}).Count(&count)
	if count != 1 {
		t.Fatalf("backfill is not idempotent: %d rows", count)
	}
}

func TestMigrateBackfillsLegacySignature(t *testing.T) {
	db := newSignatureTestDB(t)
	if err := db.Create(&User{
		Email: "legacy@example.com", Localpart: "legacy", DomainName: "example.com",
		Enabled: true, Signature: "Ada\nada@example.com",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var sig Signature
	if err := db.Where("user_email = ?", "legacy@example.com").First(&sig).Error; err != nil {
		t.Fatalf("signature not backfilled: %v", err)
	}
	if sig.BodyText != "Ada\nada@example.com" || !sig.DefaultForNew || !sig.DefaultForReply {
		t.Fatalf("unexpected backfilled signature: %+v", sig)
	}
	var applied int64
	if err := db.Model(&SchemaMigration{}).Where("id = ?", "20261023_signatures").Count(&applied).Error; err != nil {
		t.Fatalf("count migration: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration not recorded (%d rows)", applied)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var count int64
	db.Model(&Signature{}).Where("user_email = ?", "legacy@example.com").Count(&count)
	if count != 1 {
		t.Fatalf("expected one signature after re-running migrations, got %d", count)
	}
}

func TestDefaultSignatureScopePrecedence(t *testing.T) {
	db := newSignatureTestDB(t)
	seed := []Signature{
		{UserEmail: "a@example.com", Name: "generic-new", BodyText: "generic", DefaultForNew: true},
		{UserEmail: "a@example.com", Name: "generic-reply", BodyText: "reply", DefaultForReply: true},
		{UserEmail: "a@example.com", IdentityEmail: "sales@example.com", Name: "sales", BodyText: "sales", DefaultForNew: true},
		{UserEmail: "b@example.com", Name: "other", BodyText: "other", DefaultForNew: true},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed signature: %v", err)
		}
	}
	cases := []struct {
		name     string
		identity string
		reply    bool
		want     string
	}{
		{"identity-specific wins", "sales@example.com", false, "sales"},
		{"falls back to generic", "support@example.com", false, "generic"},
		{"reply uses its own default", "support@example.com", true, "reply"},
		{"owner is scoped", "a@example.com", false, "generic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig := DefaultSignature(db, "a@example.com", tc.identity, tc.reply)
			if sig == nil {
				t.Fatalf("no signature resolved")
			}
			if sig.BodyText != tc.want {
				t.Fatalf("got %q want %q", sig.BodyText, tc.want)
			}
		})
	}
	if sig := DefaultSignature(db, "a@example.com", "x@example.com", true); sig == nil || sig.BodyText != "reply" {
		t.Fatalf("unexpected reply fallback: %+v", sig)
	}
	if sig := DefaultSignature(db, "nobody@example.com", "nobody@example.com", false); sig != nil {
		t.Fatalf("expected no signature for unknown owner, got %+v", sig)
	}
}
