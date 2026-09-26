package mailflow

import (
	"strings"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

const signatureMarker = "data-mailez-signature"

const footerMarker = "data-mailez-footer"

type Options struct {
	OwnerEmail       string
	From             string
	Text, HTML       string
	Reply            bool
	SignatureApplied bool
}

func Decorate(db *gorm.DB, in Options) (string, string) {
	text, html := in.Text, in.HTML
	if !in.SignatureApplied {
		text, html = AppendSignature(db, in.OwnerEmail, in.From, text, html, in.Reply)
	}
	text, html = AppendOrgFooter(db, in.From, text, html)
	return text, html
}

func AppendSignature(db *gorm.DB, ownerEmail, identity, text, html string, reply bool) (string, string) {
	if strings.Contains(html, signatureMarker) {
		return text, html
	}
	sig := models.DefaultSignature(db, ownerEmail, identity, reply)
	if sig == nil {
		return text, html
	}
	return AppendSignatureParts(text, html, sig.BodyText, sig.BodyHTML)
}

func AppendSignatureParts(text, html, bodyText, bodyHTML string) (string, string) {
	if bodyText == "" {
		bodyText = models.SignatureHTMLToText(bodyHTML)
	}
	if bodyHTML == "" {
		bodyHTML = models.PlainTextToSignatureHTML(bodyText)
	}
	if bodyText == "" && bodyHTML == "" {
		return text, html
	}
	if strings.TrimSpace(text) != "" && bodyText != "" {
		text = strings.TrimRight(text, "\n") + "\n\n-- \n" + bodyText
	}
	if strings.TrimSpace(html) != "" && bodyHTML != "" {
		html = strings.TrimRight(html, "\n") +
			`<p><br></p><div ` + signatureMarker + `="server"><p>--</p>` + bodyHTML + `</div>`
	}
	return text, html
}

func OrgFooterFor(db *gorm.DB, domain string) *models.OrgFooter {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil
	}
	var row models.OrgFooter
	if err := db.Where("domain = ? AND enabled = ?", domain, true).First(&row).Error; err != nil {
		return nil
	}
	return &row
}

func AppendOrgFooter(db *gorm.DB, from, text, html string) (string, string) {
	if strings.Contains(html, footerMarker) {
		return text, html
	}
	row := OrgFooterFor(db, DomainOf(from))
	if row == nil {
		return text, html
	}
	return AppendFooterParts(text, html, row.BodyText, row.BodyHTML)
}

func AppendFooterParts(text, html, footerText, footerHTML string) (string, string) {
	if footerText == "" {
		footerText = models.SignatureHTMLToText(footerHTML)
	}
	if footerHTML == "" {
		footerHTML = models.PlainTextToSignatureHTML(footerText)
	}
	if footerText == "" && footerHTML == "" {
		return text, html
	}
	if strings.TrimSpace(text) != "" && footerText != "" {
		text = strings.TrimRight(text, "\n") + "\n\n" + footerText
	}
	if strings.TrimSpace(html) != "" && footerHTML != "" {
		html = strings.TrimRight(html, "\n") +
			`<p><br></p><div ` + footerMarker + `="1">` + footerHTML + `</div>`
	}
	return text, html
}

func DomainOf(address string) string {
	address = strings.TrimSpace(address)
	if at := strings.LastIndex(address, "@"); at >= 0 {
		return strings.ToLower(address[at+1:])
	}
	return ""
}
