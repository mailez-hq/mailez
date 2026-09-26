package models

import (
	"html"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

type Signature struct {
	Base
	ID              uint   `gorm:"primaryKey" json:"id"`
	UserEmail       string `gorm:"size:255;not null;index:idx_signatures_user" json:"user_email"`
	IdentityEmail   string `gorm:"size:255;not null;default:''" json:"identity_email"`
	Name            string `gorm:"size:120;not null" json:"name"`
	BodyHTML        string `gorm:"type:text;not null" json:"body_html"`
	BodyText        string `gorm:"type:text" json:"body_text"`
	DefaultForNew   bool   `gorm:"not null;default:false" json:"default_for_new"`
	DefaultForReply bool   `gorm:"not null;default:false" json:"default_for_reply"`
}

func DefaultSignature(db *gorm.DB, ownerEmail, identity string, reply bool) *Signature {
	column := "default_for_new"
	if reply {
		column = "default_for_reply"
	}
	scopes := []string{}
	if id := strings.ToLower(strings.TrimSpace(identity)); id != "" {
		scopes = append(scopes, id)
	}
	scopes = append(scopes, "")
	for _, scope := range scopes {
		var sig Signature
		if err := db.
			Where("user_email = ? AND identity_email = ? AND "+column+" = ?", ownerEmail, scope, true).
			Order("id").
			First(&sig).Error; err == nil {
			return &sig
		}
	}
	return nil
}

var (
	signatureBreak    = regexp.MustCompile(`(?i)<br\s*/?>`)
	signatureBlockEnd = regexp.MustCompile(`(?i)</(p|div|tr|li|h[1-6]|blockquote|table|pre)\s*>`)
	signatureTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	signatureBlankRun = regexp.MustCompile(`\n{3,}`)
)

func PlainTextToSignatureHTML(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			b.WriteString("<p><br></p>")
			continue
		}
		b.WriteString("<p>" + html.EscapeString(line) + "</p>")
	}
	return b.String()
}

func SignatureHTMLToText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	out := signatureBreak.ReplaceAllString(raw, "\n")
	out = signatureBlockEnd.ReplaceAllString(out, "\n")
	out = signatureTag.ReplaceAllString(out, "")
	out = html.UnescapeString(out)
	out = signatureBlankRun.ReplaceAllString(out, "\n\n")
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func backfillSignatures(db *gorm.DB) error {
	var rows []struct {
		Email     string
		Signature string
	}
	if err := db.Model(&User{}).
		Select("email", "signature").
		Where("signature <> ''").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		if strings.TrimSpace(r.Signature) == "" {
			continue
		}
		var n int64
		if err := db.Model(&Signature{}).Where("user_email = ?", r.Email).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if err := db.Create(&Signature{
			UserEmail:       r.Email,
			Name:            "Default",
			BodyHTML:        PlainTextToSignatureHTML(r.Signature),
			BodyText:        r.Signature,
			DefaultForNew:   true,
			DefaultForReply: true,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
