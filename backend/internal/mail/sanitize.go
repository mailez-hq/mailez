package mail

import (
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
