package mail

import (
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"golang.org/x/net/html"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// List-row previews: a short plain-text excerpt of each message body shown
// under the subject in the mail list. The excerpt is fetched lazily per page
// with a partial IMAP body fetch (BODY.PEEK[part]<0.N>) guided by the
// BODYSTRUCTURE already requested for the row, so list latency grows by one
// extra round-trip and ~4 KB per row instead of full body downloads.

const (
	// previewMaxBytes caps the raw fragment fetched per message.
	previewMaxBytes = 4096
	// previewMaxRunes caps the final excerpt (multi-byte safe).
	previewMaxRunes = 180
	// previewFetchTimeout bounds the whole preview fetch round-trip: previews
	// are cosmetic, so a slow or wedged IMAP server must never hold the list
	// response past this budget. On timeout the partial results are applied.
	previewFetchTimeout = 3 * time.Second
)

// preferredTextPart walks a BODYSTRUCTURE and returns the part whose content
// best represents a readable excerpt, plus its IMAP part path (empty for a
// non-multipart top-level message, where the section is BODY[TEXT]). Plain
// text beats HTML; attachment-disposition parts never count. Returns nil for
// messages with no readable text (e.g. attachments-only).
func preferredTextPart(bs *imap.BodyStructure, path []int) (*imap.BodyStructure, []int) {
	if bs == nil {
		return nil, nil
	}
	if bs.MIMEType == "multipart" {
		for _, want := range []string{"plain", "html"} {
			for i, p := range bs.Parts {
				if p == nil {
					continue
				}
				child := make([]int, len(path)+1)
				copy(child, path)
				child[len(path)] = i + 1
				if p.MIMEType == "multipart" {
					if t, tp := preferredTextPart(p, child); t != nil {
						return t, tp
					}
					continue
				}
				if p.MIMEType == "text" && p.MIMESubType == want && p.Disposition != "attachment" {
					return p, child
				}
			}
		}
		return nil, nil
	}
	if bs.MIMEType == "text" && bs.Disposition != "attachment" {
		if bs.MIMESubType == "plain" || bs.MIMESubType == "html" {
			return bs, []int{}
		}
	}
	return nil, nil
}

// previewSection builds the partial fetch for one part: at most
// previewMaxBytes from the start, without touching \Seen.
func previewSection(path []int) *imap.BodySectionName {
	bpn := imap.BodyPartName{}
	if len(path) == 0 {
		bpn.Specifier = imap.TextSpecifier
	} else {
		bpn.Path = path
	}
	return &imap.BodySectionName{
		BodyPartName: bpn,
		Peek:         true,
		Partial:      []int{0, previewMaxBytes},
	}
}

// hydratePreviews fills rows[i].Preview for every row whose raw FETCH
// response (carrying the BODYSTRUCTURE) is present in raws. Failures are
// silent: a missing preview degrades to no excerpt line, never to a failed
// list request.
func (c *Client) hydratePreviews(cli *pooledConn, rows []Message, raws map[uint32]*imap.Message) {
	if len(rows) == 0 || len(raws) == 0 {
		return
	}
	type fragment struct {
		sec      *imap.BodySectionName
		encoding string
		charset  string
		html     bool
	}
	byUID := make(map[uint32]fragment, len(rows))
	secs := make(map[string]*imap.BodySectionName, 4)
	for i := range rows {
		im := raws[rows[i].UID]
		if im == nil || im.BodyStructure == nil {
			continue
		}
		p, path := preferredTextPart(im.BodyStructure, nil)
		if p == nil {
			continue
		}
		sec := previewSection(path)
		secs[string(sec.FetchItem())] = sec
		byUID[rows[i].UID] = fragment{
			sec:      sec,
			encoding: p.Encoding,
			charset:  p.Params["charset"],
			html:     p.MIMESubType == "html",
		}
	}
	if len(byUID) == 0 {
		return
	}

	uidset := new(imap.SeqSet)
	for u := range byUID {
		uidset.AddNum(u)
	}
	items := make([]imap.FetchItem, 0, len(secs))
	for _, s := range secs {
		items = append(items, s.FetchItem())
	}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(uidset, items, messages)
	}()
	excerpts := make(map[uint32]string, len(byUID))
	timeout := time.NewTimer(previewFetchTimeout)
	defer timeout.Stop()
	timedOut := false
collect:
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				break collect
			}
			w, ok2 := byUID[msg.Uid]
			if !ok2 {
				continue
			}
			var b []byte
			if r := msg.GetBody(w.sec); r != nil {
				b, _ = io.ReadAll(io.LimitReader(r, previewMaxBytes+128))
			}
			if len(b) == 0 {
				continue
			}
			excerpts[msg.Uid] = buildPreview(b, w.encoding, w.charset, w.html)
		case <-timeout.C:
			timedOut = true
			break collect
		}
	}
	if timedOut {
		// Abandoned mid-fetch: drain the channel off-thread so the fetch
		// goroutine and its pooled connection can settle and return.
		go func() {
			for range messages {
			}
			<-done
		}()
	} else if err := <-done; err != nil {
		return
	}
	for i := range rows {
		if p, ok := excerpts[rows[i].UID]; ok && p != "" {
			rows[i].Preview = p
		}
	}
}

// buildPreview turns a raw (possibly truncated) body-part fragment into a
// single-line excerpt: transfer-decode, charset-decode, strip HTML, collapse
// whitespace and cap the length.
func buildPreview(b []byte, encoding, charset string, isHTML bool) string {
	decoded := previewDecodeTransfer(b, encoding)
	decoded = previewDecodeCharset(decoded, charset)
	if isHTML {
		decoded = []byte(htmlToText(string(decoded)))
	}
	return collapsePreview(string(decoded))
}

// previewDecodeTransfer undoes Content-Transfer-Encoding for a truncated
// fragment. Truncated base64 loses only its final (partial) group.
func previewDecodeTransfer(b []byte, encoding string) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		clean := make([]byte, 0, len(b))
		for _, c := range b {
			if c == '\r' || c == '\n' || c == ' ' || c == '\t' {
				continue
			}
			clean = append(clean, c)
		}
		if n := len(clean) / 4 * 4; n > 0 {
			if out, err := base64.StdEncoding.DecodeString(string(clean[:n])); err == nil {
				return out
			}
		}
		return b
	case "quoted-printable":
		// A fragment can end mid-soft-break ("=\r\n" cut to "="), which the
		// reader rejects; drop a trailing bare '=' so the rest decodes.
		if n := len(b); n > 0 && b[n-1] == '=' {
			b = b[:n-1]
		}
		r := quotedprintable.NewReader(strings.NewReader(string(b)))
		if out, err := io.ReadAll(r); err == nil && len(out) > 0 {
			return out
		}
		return b
	default:
		return b
	}
}

// previewDecodeCharset converts a fragment to UTF-8. The common CJK and
// western legacy charsets are covered; unknown charsets pass through
// unchanged (best-effort preview, never an error).
func previewDecodeCharset(b []byte, charset string) []byte {
	var enc encoding.Encoding
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return b
	case "gbk", "cp936", "gb_2312":
		enc = simplifiedchinese.GBK
	case "gb2312", "hz-gb-2312":
		enc = simplifiedchinese.HZGB2312
	case "gb18030":
		enc = simplifiedchinese.GB18030
	case "big5", "big5-hkscs", "big5hkscs":
		enc = traditionalchinese.Big5
	case "iso-8859-1", "latin1", "cp819":
		enc = charmap.ISO8859_1
	case "windows-1252", "cp1252":
		enc = charmap.Windows1252
	case "utf-16", "utf-16le":
		enc = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	case "utf-16be":
		enc = unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	default:
		return b
	}
	out, err := enc.NewDecoder().Bytes(b)
	if err != nil {
		if len(out) > 0 {
			return out // truncated multibyte tail: keep the decoded prefix
		}
		return b
	}
	return out
}

// htmlToText extracts visible text from an HTML fragment. Block-level tags
// become line breaks so the collapsed text reads like the rendered message.
func htmlToText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var sb strings.Builder
	atLineStart := true
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.TextToken:
			sb.WriteString(z.Token().Data)
			atLineStart = false
		case html.StartTagToken:
			name := z.Token().Data
			if name == "br" || isBlockTag(name) {
				if !atLineStart && sb.Len() > 0 {
					sb.WriteByte('\n')
					atLineStart = true
				}
			}
		}
	}
	return sb.String()
}

func isBlockTag(name string) bool {
	switch name {
	case "p", "div", "tr", "li", "table", "blockquote", "pre",
		"h1", "h2", "h3", "h4", "h5", "h6", "section", "article":
		return true
	}
	return false
}

// collapsePreview trims a decoded fragment down to the final excerpt:
// whitespace runs become single spaces, the result is capped at
// previewMaxRunes with an ellipsis.
func collapsePreview(s string) string {
	out := strings.Join(strings.Fields(s), " ")
	r := []rune(out)
	if len(r) > previewMaxRunes {
		return string(r[:previewMaxRunes]) + "…"
	}
	return out
}
