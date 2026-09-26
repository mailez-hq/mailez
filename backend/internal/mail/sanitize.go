package mail

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// sanitizeHTMLPolicy strips executable content from untrusted email HTML
// (scripts, event handlers, forms, embedded objects) while keeping the
// formatting email clients actually render. It mirrors the webmail's
// DOMPurify allowlist so both sides agree on what is safe.
func sanitizeHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements(
		"p", "br", "span", "div",
		"b", "i", "em", "strong", "u", "s", "sub", "sup",
		"blockquote", "pre", "code",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tr", "th", "td",
		"a", "img",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"hr", "cite", "del", "ins", "mark", "small",
	)
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt", "width", "height").OnElements("img")
	p.AllowAttrs("colspan", "rowspan", "align", "width", "border", "cellpadding", "cellspacing").OnElements("td", "th", "table")
	p.AllowURLSchemes("http", "https", "mailto", "cid", "data")
	return p
}

// SanitizeHTML cleans untrusted email HTML. Defense in depth: the webmail
// also sanitizes on render, but every other consumer of the API gets safe
// HTML from here.
func SanitizeHTML(html string) string {
	if html == "" {
		return ""
	}
	return sanitizeHTMLPolicy().Sanitize(html)
}

func sanitizeSignaturePolicy() *bluemonday.Policy {
	p := sanitizeHTMLPolicy()
	p.AllowAttrs("style").Globally()
	p.AllowAttrs("bgcolor").OnElements("td", "th", "table")
	return p
}

func SanitizeSignatureHTML(html string) string {
	if html == "" {
		return ""
	}
	return sanitizeSignatureStyles(sanitizeSignaturePolicy().Sanitize(html))
}

var signatureStyleAttr = regexp.MustCompile(`(?i)\sstyle\s*=\s*"([^"]*)"`)

var signatureStyleAllowedProps = map[string]bool{
	"color": true, "background": true, "background-color": true,
	"font-family": true, "font-size": true, "font-style": true, "font-weight": true,
	"text-align": true, "text-decoration": true, "text-transform": true, "line-height": true,
	"letter-spacing": true, "vertical-align": true, "white-space": true,
	"border": true, "border-top": true, "border-right": true, "border-bottom": true,
	"border-left": true, "border-color": true, "border-style": true, "border-width": true,
	"border-radius": true, "border-collapse": true, "border-spacing": true,
	"padding": true, "padding-top": true, "padding-right": true, "padding-bottom": true, "padding-left": true,
	"margin": true, "margin-top": true, "margin-right": true, "margin-bottom": true, "margin-left": true,
	"width": true, "height": true, "max-width": true, "min-width": true,
}

var signatureStyleUnsafeValue = regexp.MustCompile(
	`(?i)(url\s*\(|expression\s*\(|javascript:|vbscript:|@import|<|>|\\|/\*)`,
)

func sanitizeSignatureStyles(html string) string {
	return signatureStyleAttr.ReplaceAllStringFunc(html, func(match string) string {
		parts := signatureStyleAttr.FindStringSubmatch(match)
		kept := filterSignatureStyle(parts[1])
		if kept == "" {
			return ""
		}
		return ` style="` + kept + `"`
	})
}

func filterSignatureStyle(decls string) string {
	kept := make([]string, 0, 4)
	for _, decl := range strings.Split(decls, ";") {
		prop, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		value = strings.TrimSpace(value)
		if !signatureStyleAllowedProps[prop] || value == "" {
			continue
		}
		if signatureStyleUnsafeValue.MatchString(value) {
			continue
		}
		kept = append(kept, prop+": "+value)
	}
	return strings.Join(kept, "; ")
}
