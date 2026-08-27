// Package webdav implements the transport layer shared by the built-in
// CardDAV and CalDAV servers. It speaks the WebDAV method subset that real
// clients (iOS, Android DAVx5, Thunderbird, Outlook) actually use:
// OPTIONS, PROPFIND (allprop/prop/propname), GET/HEAD, PUT, DELETE and
// REPORT. LOCK/COPY/MOVE are deliberately omitted — none of the mainstream
// DAV clients require them for address books or calendars.
//
// The package is protocol-only: a Backend implementation maps paths to
// storage (contacts / calendar events) and renders properties. Property
// values are built with the El/Empty helpers so escaping stays consistent.
package webdav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// DAV XML namespaces used across the transport and the CardDAV/CalDAV
// extensions.
const (
	NSDAV     = "DAV:"
	NSCardDAV = "urn:ietf:params:xml:ns:carddav"
	NSCalDAV  = "urn:ietf:params:xml:ns:caldav"
	NSApple   = "http://apple.com/ns/ical/"
)

// nsPrefix maps a namespace to the prefix used in generated XML. Unknown
// namespaces fall back to the full URI.
var nsPrefix = map[string]string{
	NSDAV:     "D",
	NSCardDAV: "C",
	NSCalDAV:  "CA",
	NSApple:   "A",
}

// Depth is the PROPFIND/REPORT traversal depth.
type Depth int

const (
	DepthZero     Depth = 0
	DepthOne      Depth = 1
	DepthInfinity Depth = 2 // collapsed to DepthOne; collections are shallow
)

// Prop is one property in a PROPFIND/REPORT response. Value is the inner XML
// of the property element (use Text for text values, Href for hrefs, Empty
// for nested empty elements); Status defaults to 200.
type Prop struct {
	XMLName xml.Name
	Value   string
	Status  int
}

// Response is one <response> element of a multistatus reply.
type Response struct {
	Href   string
	Status int // response-level status (0 = per-property propstats)
	Props  []Prop
}

// Error carries an HTTP status code and optional DAV error detail for
// precondition failures.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

// HTTPError builds a webdav.Error with a status code.
func HTTPError(status int, format string, args ...any) error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// Backend is the storage surface a DAV server dispatches to. Paths are
// URL-decoded and start with "/dav". Implementations decide whether a path
// names a principal, a collection or a resource.
type Backend interface {
	// PropFind returns the multistatus responses for path at the given
	// depth. props is nil for allprop, an empty non-nil slice for propname,
	// or the explicitly requested property names.
	PropFind(ctx context.Context, path string, props []xml.Name, depth Depth) ([]Response, error)
	// Get returns the resource body, content type and ETag.
	Get(ctx context.Context, path string) (body []byte, contentType, etag string, err error)
	// Put stores a resource body. ifMatch/ifNoneMatch carry the raw header
	// values (may be "*" or an ETag). Returns the new ETag and whether the
	// resource was created.
	Put(ctx context.Context, path string, body []byte, ifMatch, ifNoneMatch string) (etag string, created bool, err error)
	// Delete removes a resource.
	Delete(ctx context.Context, path string) error
	// Report dispatches REPORT requests; reportName is the top-level element
	// (e.g. carddav:addressbook-query) and body is the raw report payload.
	Report(ctx context.Context, path string, reportName xml.Name, body []byte) ([]Response, error)
}

// Server adapts a Backend to Fiber and renders protocol responses.
type Server struct {
	Backend      Backend
	Capabilities []string // e.g. "addressbook", "calendar-access"
}

type ctxKey int

const userCtxKey ctxKey = iota

// WithUser carries the authenticated DAV user in the request context so
// backends can render discovery properties without re-parsing credentials.
func WithUser(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, userCtxKey, email)
}

// UserFrom returns the authenticated DAV user, or "" when absent.
func UserFrom(ctx context.Context) string {
	v, _ := ctx.Value(userCtxKey).(string)
	return v
}

// DAVHeader returns the DAV compliance header value.
func (s *Server) DAVHeader() string {
	caps := append([]string{"1", "3"}, s.Capabilities...)
	return strings.Join(caps, ", ")
}

// Handle is the Fiber entrypoint mounted at /dav.
func (s *Server) Handle(c *fiber.Ctx) error {
	path := c.Path()
	switch c.Method() {
	case http.MethodOptions:
		c.Set("DAV", s.DAVHeader())
		c.Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
		c.Set("Accept-Ranges", "bytes")
		return c.SendStatus(http.StatusNoContent)
	case http.MethodHead, http.MethodGet:
		body, ctype, etag, err := s.Backend.Get(c.UserContext(), path)
		if err != nil {
			return serveError(c, err)
		}
		if ctype != "" {
			c.Set("Content-Type", ctype)
		}
		if etag != "" {
			c.Set("ETag", quoteETag(etag))
		}
		c.Set("Content-Length", strconv.Itoa(len(body)))
		if c.Method() == http.MethodHead {
			return c.SendStatus(http.StatusOK)
		}
		return c.Send(body)
	case "PROPFIND":
		mode, props, err := parsePropFind(c.Body())
		if err != nil {
			return serveError(c, HTTPError(http.StatusBadRequest, "bad PROPFIND body: %v", err))
		}
		if mode == "propname" {
			props = []xml.Name{} // non-nil empty: ask backend for names only
		}
		if mode == "allprop" {
			props = nil
		}
		depth := parseDepth(c.Get("Depth"), DepthInfinity)
		resps, err := s.Backend.PropFind(c.UserContext(), path, props, depth)
		if err != nil {
			return serveError(c, err)
		}
		return c.Status(http.StatusMultiStatus).Type("application/xml; charset=utf-8").Send(RenderMultiStatus(resps))
	case http.MethodPut:
		body := c.Body()
		etag, created, err := s.Backend.Put(c.UserContext(), path, body, c.Get("If-Match"), c.Get("If-None-Match"))
		if err != nil {
			return serveError(c, err)
		}
		if etag != "" {
			c.Set("ETag", quoteETag(etag))
		}
		if created {
			return c.SendStatus(http.StatusCreated)
		}
		return c.SendStatus(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.Backend.Delete(c.UserContext(), path); err != nil {
			return serveError(c, err)
		}
		return c.SendStatus(http.StatusNoContent)
	case "REPORT":
		name, err := reportName(c.Body())
		if err != nil {
			return serveError(c, HTTPError(http.StatusBadRequest, "bad REPORT body: %v", err))
		}
		depth := parseDepth(c.Get("Depth"), DepthZero)
		resps, err := s.Backend.Report(c.UserContext(), path, name, c.Body())
		if err != nil {
			return serveError(c, err)
		}
		_ = depth
		return c.Status(http.StatusMultiStatus).Type("application/xml; charset=utf-8").Send(RenderMultiStatus(resps))
	default:
		return serveError(c, HTTPError(http.StatusMethodNotAllowed, "method not supported"))
	}
}

// ServePrincipal answers PROPFIND/OPTIONS on principal URLs with the
// current-user-principal and the addressbook/calendar home sets.
func ServePrincipal(c *fiber.Ctx, principalPath string, addressbookHome, calendarHome string) error {
	switch c.Method() {
	case http.MethodOptions:
		c.Set("DAV", "1, 3, addressbook, calendar-access")
		c.Set("Allow", "OPTIONS, PROPFIND")
		return c.SendStatus(http.StatusNoContent)
	case "PROPFIND":
		props := []Prop{
			{XMLName: xml.Name{Space: NSDAV, Local: "resourcetype"}, Value: El(xml.Name{Space: NSDAV, Local: "principal"}, "")},
			{XMLName: xml.Name{Space: NSDAV, Local: "current-user-principal"}, Value: Href(principalPath)},
		}
		if addressbookHome != "" {
			props = append(props, Prop{
				XMLName: xml.Name{Space: NSCardDAV, Local: "addressbook-home-set"},
				Value:   Href(addressbookHome),
			})
		}
		if calendarHome != "" {
			props = append(props, Prop{
				XMLName: xml.Name{Space: NSCalDAV, Local: "calendar-home-set"},
				Value:   Href(calendarHome),
			})
		}
		return c.Status(http.StatusMultiStatus).Type("application/xml; charset=utf-8").
			Send(RenderMultiStatus([]Response{{Href: c.Path(), Props: props}}))
	default:
		return serveError(c, HTTPError(http.StatusMethodNotAllowed, "method not supported"))
	}
}

// El renders <prefix:local>content</prefix:local> with escaped content.
func El(name xml.Name, content string) string {
	return "<" + RenderName(name) + ">" + xmlEscape(content) + "</" + RenderName(name) + ">"
}

// Empty renders a self-closing <prefix:local/> element.
func Empty(name xml.Name) string {
	return "<" + RenderName(name) + "/>"
}

// Href renders a DAV href property value.
func Href(path string) string {
	return El(xml.Name{Space: NSDAV, Local: "href"}, path)
}

// RenderName renders an XML name with the namespace prefix table.
func RenderName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	if p, ok := nsPrefix[name.Space]; ok {
		return p + ":" + name.Local
	}
	return name.Local
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Text renders a property value as escaped text (inner XML).
func Text(s string) string {
	return xmlEscape(s)
}

// RenderMultiStatus serializes responses into a DAV multistatus document.
func RenderMultiStatus(resps []Response) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav" `)
	b.WriteString(`xmlns:CA="urn:ietf:params:xml:ns:caldav" xmlns:A="http://apple.com/ns/ical/">`)
	for _, r := range resps {
		b.WriteString("<D:response><D:href>")
		b.WriteString(xmlEscape(r.Href))
		b.WriteString("</D:href>")
		if r.Status != 0 {
			b.WriteString("<D:status>")
			b.WriteString(httpStatusLine(r.Status))
			b.WriteString("</D:status>")
		} else {
			// Group properties by status.
			groups := map[int][]Prop{}
			var order []int
			for _, p := range r.Props {
				st := p.Status
				if st == 0 {
					st = http.StatusOK
				}
				if _, ok := groups[st]; !ok {
					order = append(order, st)
				}
				groups[st] = append(groups[st], p)
			}
			for _, st := range order {
				b.WriteString("<D:propstat><D:prop>")
				for _, p := range groups[st] {
					b.WriteString("<" + RenderName(p.XMLName) + ">")
					b.WriteString(p.Value)
					b.WriteString("</" + RenderName(p.XMLName) + ">")
				}
				b.WriteString("</D:prop><D:status>")
				b.WriteString(httpStatusLine(st))
				b.WriteString("</D:status></D:propstat>")
			}
		}
		b.WriteString("</D:response>")
	}
	b.WriteString("</D:multistatus>")
	return []byte(b.String())
}

func httpStatusLine(code int) string {
	return fmt.Sprintf("HTTP/1.1 %d %s", code, http.StatusText(code))
}

// parseDepth parses the Depth header, defaulting when absent.
func parseDepth(v string, def Depth) Depth {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0":
		return DepthZero
	case "1":
		return DepthOne
	case "infinity":
		return DepthInfinity
	default:
		return def
	}
}

// parsePropFind inspects a PROPFIND body and returns the requested mode and
// property names (nil = allprop, empty non-nil = propname).
func parsePropFind(body []byte) (string, []xml.Name, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "allprop", nil, nil
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	mode := ""
	var props []xml.Name
	inProp := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "allprop":
				mode = "allprop"
			case "propname":
				mode = "propname"
			case "prop":
				inProp = true
			default:
				if inProp {
					props = append(props, t.Name)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "prop" {
				inProp = false
			}
		}
	}
	if mode == "" {
		mode = "prop"
	}
	return mode, props, nil
}

// reportName extracts the top-level REPORT element name.
func reportName(body []byte) (xml.Name, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.Name{}, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name, nil
		}
	}
}

// quoteETag ensures an ETag carries surrounding double quotes.
func quoteETag(etag string) string {
	if strings.HasPrefix(etag, "\"") && strings.HasSuffix(etag, "\"") {
		return etag
	}
	return `"` + etag + `"`
}

// ETagMatches evaluates an If-Match/If-None-Match header against an ETag
// (both may carry quotes; "*" matches any existing resource).
func ETagMatches(header, etag string) bool {
	if strings.TrimSpace(header) == "*" {
		return true
	}
	want := strings.TrimSpace(header)
	if want == "" {
		return true
	}
	want = strings.Trim(want, `"`)
	return strings.Trim(etag, `"`) == want
}

func serveError(c *fiber.Ctx, err error) error {
	var de *Error
	if ok := asError(err, &de); ok {
		msg := de.Message
		if msg == "" {
			msg = http.StatusText(de.Status)
		}
		return c.Status(de.Status).Type("text/plain").SendString(msg)
	}
	// Unknown error kinds are internal failures (storage, DB, …): log the
	// details server-side and report a generic message so internals never
	// reach DAV clients.
	log.Printf("webdav internal error (%s %s): %v", c.Method(), c.Path(), err)
	return c.Status(http.StatusInternalServerError).Type("text/plain").SendString(http.StatusText(http.StatusInternalServerError))
}

func asError(err error, target **Error) bool {
	if e, ok := err.(*Error); ok {
		*target = e
		return true
	}
	return false
}
