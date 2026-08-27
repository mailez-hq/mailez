// Package dlp implements outbound content filtering and approval
// (Coremail-style 敏感词过滤 + 审批): rules scan outbound mail and either
// block it at submission or hold it for an approver's decision.
package dlp

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"

	"mailez/backend/internal/core/models"
)

// scanText builds the searchable text of a message: headers + text parts
// (HTML stripped). Attachment content is not scanned in v1 (the attachment
// full-text index covers that separately).
func scanText(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}
	var out strings.Builder
	for _, key := range []string{"From", "To", "Cc", "Subject"} {
		if v := msg.Header.Get(key); v != "" {
			out.WriteString(key)
			out.WriteString(": ")
			out.WriteString(v)
			out.WriteByte('\n')
		}
	}
	writeTextParts(msg.Header, msg.Body, &out)
	return out.String()
}

// renderPreview extracts just the message body text for the review pane.
func renderPreview(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	var out strings.Builder
	writeTextParts(msg.Header, msg.Body, &out)
	return strings.TrimSpace(out.String())
}

func writeTextParts(h mail.Header, body io.Reader, out *strings.Builder) {
	mediaType, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		mediaType = "text/plain"
	}
	if strings.HasPrefix(mediaType, "text/") {
		b, _ := io.ReadAll(io.LimitReader(body, 1<<20))
		out.WriteString(decodeText(h.Get("Content-Transfer-Encoding"), b))
		return
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return
	}
	mr := multipart.NewReader(body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			return
		}
		pt, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if strings.HasPrefix(pt, "text/") {
			b, _ := io.ReadAll(io.LimitReader(p, 1<<20))
			out.WriteString(decodeText(p.Header.Get("Content-Transfer-Encoding"), b))
			out.WriteByte('\n')
		}
		p.Close()
	}
}

func decodeText(cte string, b []byte) string {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		if dec, err := base64.StdEncoding.DecodeString(string(b)); err == nil {
			return stripHTML(dec)
		}
	case "quoted-printable":
		if dec, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(b))); err == nil {
			return stripHTML(dec)
		}
	}
	return stripHTML(b)
}

var (
	tagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRe  = regexp.MustCompile(`\s+`)
	boundary = regexp.MustCompile(`(?m)^--[0-9A-Za-z]+`)
)

func stripHTML(b []byte) string {
	s := string(b)
	// Drop base64 MIME part boundaries that sometimes leak into quoted
	// encodings.
	s = boundary.ReplaceAllString(s, " ")
	s = tagRe.ReplaceAllString(s, " ")
	return spaceRe.ReplaceAllString(s, " ")
}

// buildMatcher compiles the effective rules for a sender domain into one
// combined regex plus the rule index, so one pass scans all patterns.
func buildMatcher(rules []models.DlpRule) (*regexp.Regexp, map[int]int, error) {
	var parts []string
	index := map[int]int{} // regexp group -> rule index
	group := 0
	for i, r := range rules {
		if !r.Enabled {
			continue
		}
		var re string
		if r.IsRegex {
			re = r.Pattern
		} else {
			re = regexp.QuoteMeta(r.Pattern)
		}
		parts = append(parts, "("+re+")")
		group++
		index[group] = i
	}
	if len(parts) == 0 {
		return nil, nil, nil
	}
	re, err := regexp.Compile("(?i)" + strings.Join(parts, "|"))
	if err != nil {
		return nil, nil, err
	}
	return re, index, nil
}

// ruleApplies reports whether a rule is in scope for the sender domain.
func ruleApplies(r models.DlpRule, domain string) bool {
	if r.Scope == "" || r.Scope == "all" {
		return true
	}
	if d, ok := strings.CutPrefix(r.Scope, "domain:"); ok {
		return strings.EqualFold(strings.TrimSpace(d), domain)
	}
	return false
}
