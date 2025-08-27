package contacts

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"mailez/backend/internal/core/models"
)

// newContactUID generates a random vCard UID for locally-created contacts so
// the built-in CardDAV server can expose every row as a stable resource.
func newContactUID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "mailez-" + time.Now().Format("20060102150405")
	}
	return "mailez-" + hex.EncodeToString(b)
}

// Minimal vCard 3.0 parser/writer for address-book import/export. We only
// round-trip the fields the Contact model stores: name, email, comment,
// groups (CATEGORIES) and avatar (PHOTO;VALUE=URI).

// ParsedContact is a contact extracted from an incoming vCard stream.
type ParsedContact struct {
	Name    string
	Email   string
	Comment string
	Groups  string
	Avatar  string
	UID     string
}

// ParseVCard parses one or more vCard 3.0/4.0 cards into contacts.
func ParseVCard(data string) []ParsedContact {
	var out []ParsedContact
	for _, card := range splitCards(data) {
		fields := parseCard(card)
		if fields.email == "" && fields.name == "" {
			continue
		}
		out = append(out, ParsedContact{
			Name:    fields.name,
			Email:   fields.email,
			Comment: fields.comment,
			Groups:  fields.groups,
			Avatar:  fields.avatar,
			UID:     fields.uid,
		})
	}
	return out
}

// EncodeVCard renders contacts as a vCard 3.0 stream.
func EncodeVCard(contacts []models.Contact) string {
	var b strings.Builder
	for _, c := range contacts {
		b.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\n")
		if c.DavUID != "" {
			b.WriteString("UID:" + vcardEscape(c.DavUID) + "\r\n")
		}
		if c.Name != "" {
			b.WriteString("FN:" + vcardEscape(c.Name) + "\r\n")
		}
		if c.Email != "" {
			b.WriteString("EMAIL;TYPE=INTERNET:" + vcardEscape(c.Email) + "\r\n")
		}
		if c.Comment != "" {
			b.WriteString("NOTE:" + vcardEscape(c.Comment) + "\r\n")
		}
		if c.Groups != "" {
			b.WriteString("CATEGORIES:" + vcardEscape(c.Groups) + "\r\n")
		}
		if c.Avatar != "" && isURIAvatar(c.Avatar) {
			b.WriteString("PHOTO;VALUE=URI:" + vcardEscape(c.Avatar) + "\r\n")
		}
		if c.DavRev > 0 && !c.UpdatedAt.IsZero() {
			b.WriteString("REV:" + c.UpdatedAt.UTC().Format("20060102T150405Z") + "\r\n")
		}
		b.WriteString("END:VCARD\r\n")
	}
	return b.String()
}

type vcardFields struct {
	name    string
	email   string
	comment string
	groups  string
	avatar  string
	uid     string
}

// splitCards splits raw text into individual vCard blocks.
func splitCards(data string) []string {
	var cards []string
	var cur strings.Builder
	inCard := false
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		upper := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, "BEGIN:VCARD") {
			inCard = true
			cur.Reset()
			continue
		}
		if inCard && strings.HasPrefix(upper, "END:VCARD") {
			cards = append(cards, cur.String())
			inCard = false
			continue
		}
		if inCard {
			cur.WriteString(line)
			cur.WriteString("\n")
		}
	}
	return cards
}

// parseCard extracts known fields from a single unfolded vCard block.
func parseCard(card string) vcardFields {
	var f vcardFields
	var curName, curValue string
	unfolded := unfoldVCard(card)
	for _, line := range unfolded {
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		curName = strings.ToUpper(strings.TrimSpace(line[:colon]))
		curValue = strings.TrimSpace(line[colon+1:])
		switch {
		case strings.HasPrefix(curName, "UID"):
			if f.uid == "" {
				f.uid = vcardUnescape(curValue)
			}
		case strings.HasPrefix(curName, "FN"):
			if f.name == "" {
				f.name = vcardUnescape(curValue)
			}
		case strings.HasPrefix(curName, "EMAIL"):
			if f.email == "" {
				f.email = vcardUnescape(curValue)
			}
		case strings.HasPrefix(curName, "NOTE"):
			if f.comment == "" {
				f.comment = vcardUnescape(curValue)
			}
		case strings.HasPrefix(curName, "CATEGORIES"):
			if f.groups == "" {
				f.groups = vcardUnescape(curValue)
			}
		case strings.HasPrefix(curName, "PHOTO"):
			// Only accept URI photos; skip inline base64 blobs.
			if f.avatar == "" && (strings.Contains(curName, "VALUE=URI") || isURIAvatar(curValue)) {
				f.avatar = vcardUnescape(curValue)
			}
		}
	}
	return f
}

// unfoldVCard joins continuation lines (leading space/tab) with their parent.
func unfoldVCard(card string) []string {
	lines := strings.Split(card, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" && (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += line[1:]
			continue
		}
		out = append(out, line)
	}
	return out
}

// vcardUnescape reverses the vCard backslash escapes.
func vcardUnescape(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\,`, ",")
	s = strings.ReplaceAll(s, `\;`, ";")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// vcardEscape escapes a value for a vCard text line.
func vcardEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, ";", `\;`)
	return s
}

// isURIAvatar reports whether an avatar value is a URL or data URI rather
// than a raw inline image payload.
func isURIAvatar(s string) bool {
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "http://") ||
		strings.HasPrefix(low, "https://") ||
		strings.HasPrefix(low, "data:")
}
