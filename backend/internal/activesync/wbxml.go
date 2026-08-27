package activesync

// WBXML codec for Exchange ActiveSync ([MS-ASWBXML]).
//
// EAS uses the WAP binary XML container with two quirks that differ from
// generic WAP WBXML: the content bit is 0x40 (attributes 0x80, the reverse
// of the WAP convention) and inline strings are null-terminated rather than
// length-prefixed. Real Exchange captures and the reference implementations
// (activesync-go, Z-Push) all follow these rules.
import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// WBXML global tokens.
const (
	wbSwitchPage    byte = 0x00
	wbEnd           byte = 0x01
	wbEntity        byte = 0x02
	wbStrI          byte = 0x03
	wbLiteral       byte = 0x04
	wbExtI0         byte = 0x40
	wbExtI1         byte = 0x41
	wbExtI2         byte = 0x42
	wbPI            byte = 0x43
	wbLiteralC      byte = 0x44
	wbStrT          byte = 0x83
	wbLiteralA      byte = 0x84
	wbOpaque        byte = 0xC3
	wbLiteralAC     byte = 0xC4
	wbTagContentBit byte = 0x40
	wbTagAttrBit    byte = 0x80
	wbTagMask       byte = 0x3F
	wbVersion       byte = 0x03
	wbPublicUnknown byte = 0x01
	wbCharsetUTF8   byte = 0x6A
)

// Attr is one attribute of an element (rare in EAS; decoded for tolerance).
type Attr struct {
	Name  string
	Value string
}

// Element is one node of the WBXML document tree. NS is the code page
// namespace (e.g. "AirSync", "AirSyncBase"); Name is the tag name.
type Element struct {
	NS       string
	Name     string
	Text     string
	Attrs    []Attr
	Children []*Element
}

// Add appends a child element and returns it.
func (e *Element) Add(ns, name, text string) *Element {
	c := &Element{NS: ns, Name: name, Text: text}
	e.Children = append(e.Children, c)
	return c
}

// Child returns the first child with the given namespace and name.
func (e *Element) Child(ns, name string) *Element {
	for _, c := range e.Children {
		if c.NS == ns && c.Name == name {
			return c
		}
	}
	return nil
}

// ChildText returns the text of the first matching child, or "".
func (e *Element) ChildText(ns, name string) string {
	if c := e.Child(ns, name); c != nil {
		return c.Text
	}
	return ""
}

// ChildrenNamed returns every child with the given namespace and name.
func (e *Element) ChildrenNamed(ns, name string) []*Element {
	var out []*Element
	for _, c := range e.Children {
		if c.NS == ns && c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// String renders the tree as XML for debugging and tests.
func (e *Element) String() string {
	var b strings.Builder
	e.writeXML(&b, 0)
	return b.String()
}

func (e *Element) writeXML(b *strings.Builder, depth int) {
	pad := strings.Repeat("  ", depth)
	name := e.Name
	if e.NS != "" && e.NS != "AirSync" {
		name = e.NS + ":" + e.Name
	}
	if e.Text == "" && len(e.Children) == 0 {
		b.WriteString(pad + "<" + name + "/>\n")
		return
	}
	b.WriteString(pad + "<" + name + ">")
	if e.Text != "" {
		b.WriteString(escapeXML(e.Text))
	}
	if len(e.Children) > 0 {
		b.WriteString("\n")
		for _, c := range e.Children {
			c.writeXML(b, depth+1)
		}
		b.WriteString(pad)
	}
	b.WriteString("</" + name + ">\n")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ---------------------------------------------------------------------------
// Encoder
// ---------------------------------------------------------------------------

type wbxmlEncoder struct {
	buf     bytes.Buffer
	book    []*codePageDef
	current byte
}

// EncodeWBXML serializes the document tree into a WBXML byte stream using
// the full EAS code book.
func EncodeWBXML(root *Element) ([]byte, error) {
	enc := &wbxmlEncoder{book: easCodePages}
	enc.buf.WriteByte(wbVersion)
	enc.writeMBUint32(uint32(wbPublicUnknown))
	enc.writeMBUint32(uint32(wbCharsetUTF8))
	enc.writeMBUint32(0) // string table: EAS clients keep it empty
	if err := enc.element(root, true); err != nil {
		return nil, err
	}
	return enc.buf.Bytes(), nil
}

func (e *wbxmlEncoder) element(el *Element, isRoot bool) error {
	if el.Name == "" {
		return errors.New("wbxml: element without a name")
	}
	if len(el.Attrs) > 0 {
		return errors.New("wbxml: attribute emission is not supported")
	}
	cp, err := e.pageFor(el.NS)
	if err != nil {
		return err
	}
	tok, ok := cp.byName[el.Name]
	if !ok {
		return fmt.Errorf("wbxml: no tag %s in code page %s", el.Name, el.NS)
	}
	if err := e.switchPage(cp.index); err != nil {
		return err
	}
	hasContent := el.Text != "" || len(el.Children) > 0
	var flags byte
	if hasContent {
		flags |= wbTagContentBit
	}
	e.buf.WriteByte(flags | tok)
	if !hasContent {
		return nil
	}
	if el.Text != "" {
		if err := e.writeString(el.Text); err != nil {
			return err
		}
	}
	for _, c := range el.Children {
		if err := e.element(c, false); err != nil {
			return err
		}
	}
	e.buf.WriteByte(wbEnd)
	return nil
}

func (e *wbxmlEncoder) pageFor(ns string) (*codePageDef, error) {
	if ns == "" {
		return e.book[0], nil
	}
	for _, cp := range e.book {
		if cp.ns == ns {
			return cp, nil
		}
	}
	return nil, fmt.Errorf("wbxml: unknown code page %q", ns)
}

func (e *wbxmlEncoder) switchPage(idx byte) error {
	if e.current == idx {
		return nil
	}
	if pageByIndex(e.book, idx) == nil {
		return fmt.Errorf("wbxml: code page index %d out of range", idx)
	}
	e.buf.WriteByte(wbSwitchPage)
	e.buf.WriteByte(idx)
	e.current = idx
	return nil
}

// pageByIndex finds the code page with the given protocol index.
func pageByIndex(book []*codePageDef, idx byte) *codePageDef {
	for _, cp := range book {
		if cp.index == idx {
			return cp
		}
	}
	return nil
}

func (e *wbxmlEncoder) writeString(s string) error {
	e.buf.WriteByte(wbStrI)
	if strings.IndexByte(s, 0) >= 0 {
		return errors.New("wbxml: string contains NUL")
	}
	e.buf.WriteString(s)
	e.buf.WriteByte(0)
	return nil
}

// writeMBUint32 writes a WBXML multibyte integer (7 bits per byte, high bit
// set on every byte but the last).
func (e *wbxmlEncoder) writeMBUint32(v uint32) {
	var tmp [5]byte
	i := 4
	tmp[i] = byte(v & 0x7F)
	v >>= 7
	for v > 0 {
		i--
		tmp[i] = byte(v&0x7F) | 0x80
		v >>= 7
	}
	e.buf.Write(tmp[i:])
}

// ---------------------------------------------------------------------------
// Decoder
// ---------------------------------------------------------------------------

type wbxmlDecoder struct {
	r       *bufio.Reader
	book    []*codePageDef
	current byte
	table   []byte
}

// DecodeWBXML parses a WBXML byte stream into an element tree.
func DecodeWBXML(data []byte, book []*codePageDef) (*Element, error) {
	if book == nil {
		book = easCodePages
	}
	d := &wbxmlDecoder{r: bufio.NewReader(bytes.NewReader(data)), book: book}
	if err := d.header(); err != nil {
		return nil, err
	}
	d.current = 0
	return d.element()
}

func (d *wbxmlDecoder) header() error {
	version, err := d.r.ReadByte()
	if err != nil {
		return err
	}
	if version > wbVersion {
		return fmt.Errorf("wbxml: unsupported version 0x%02x", version)
	}
	if _, err := d.readMBUint32(); err != nil { // public identifier
		return err
	}
	if _, err := d.readMBUint32(); err != nil { // charset
		return err
	}
	n, err := d.readMBUint32()
	if err != nil {
		return err
	}
	if n > 1<<20 {
		return errors.New("wbxml: string table too large")
	}
	d.table = make([]byte, n)
	if _, err := io.ReadFull(d.r, d.table); err != nil {
		return err
	}
	return nil
}

// readMBUint32 reads a WBXML multibyte integer.
func (d *wbxmlDecoder) readMBUint32() (uint32, error) {
	var v uint32
	for i := 0; i < 5; i++ {
		b, err := d.r.ReadByte()
		if err != nil {
			return 0, err
		}
		v = v<<7 | uint32(b&0x7F)
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, errors.New("wbxml: multibyte integer too long")
}

// element parses one element (including any leading SWITCH_PAGE tokens).
func (d *wbxmlDecoder) element() (*Element, error) {
	tok, err := d.r.ReadByte()
	if err != nil {
		return nil, err
	}
	for tok == wbSwitchPage {
		page, err := d.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if pageByIndex(d.book, page) == nil {
			return nil, fmt.Errorf("wbxml: unknown code page %d", page)
		}
		d.current = page
		tok, err = d.r.ReadByte()
		if err != nil {
			return nil, err
		}
	}
	// Global tokens that can precede a tag.
	switch tok {
	case wbStrI:
		s, err := d.readString()
		if err != nil {
			return nil, err
		}
		return &Element{Name: s}, nil
	case wbLiteral, wbLiteralC, wbLiteralA, wbLiteralAC:
		name, err := d.readString()
		if err != nil {
			return nil, err
		}
		el := &Element{Name: name}
		if tok == wbLiteralA || tok == wbLiteralAC {
			if err := d.skipAttributes(); err != nil {
				return nil, err
			}
		}
		if tok == wbLiteralC || tok == wbLiteralAC {
			content, err := d.content()
			if err != nil {
				return nil, err
			}
			el.Text = content.text
			el.Children = content.children
		}
		return el, nil
	}
	// Remaining global tokens (EXT_*, PI, STR_T, OPAQUE) all carry a tag code
	// of 0x00-0x04; real EAS tags start at 0x05. Distinguishing by code keeps
	// tags that combine content+attribute flags (0xC0|code) valid.
	code := tok & wbTagMask
	if code < 0x05 {
		switch tok {
		case wbEnd:
			return nil, errors.New("wbxml: unexpected END token")
		case wbEntity:
			return nil, errors.New("wbxml: unexpected ENTITY token")
		case wbOpaque:
			return nil, errors.New("wbxml: unexpected OPAQUE token")
		case wbStrT:
			return nil, errors.New("wbxml: unexpected STR_T token")
		}
		return nil, fmt.Errorf("wbxml: unexpected token 0x%02x", tok)
	}
	cp := pageByIndex(d.book, d.current)
	if cp == nil {
		return nil, fmt.Errorf("wbxml: current code page %d not in code book", d.current)
	}
	name, ok := cp.byToken[code]
	if !ok {
		return nil, fmt.Errorf("wbxml: unknown tag 0x%02x in code page %d (%s)", code, d.current, cp.ns)
	}
	el := &Element{NS: cp.ns, Name: name}
	if tok&wbTagAttrBit != 0 {
		attrs, err := d.attributes()
		if err != nil {
			return nil, err
		}
		el.Attrs = attrs
	}
	if tok&wbTagContentBit != 0 {
		content, err := d.content()
		if err != nil {
			return nil, err
		}
		el.Text = content.text
		el.Children = content.children
	}
	return el, nil
}

type decodedContent struct {
	text     string
	children []*Element
}

// content parses tokens until the matching END token.
func (d *wbxmlDecoder) content() (decodedContent, error) {
	var out decodedContent
	for {
		tok, err := d.r.ReadByte()
		if err != nil {
			return out, err
		}
		switch {
		case tok == wbEnd:
			return out, nil
		case tok == wbSwitchPage:
			page, err := d.r.ReadByte()
			if err != nil {
				return out, err
			}
			if pageByIndex(d.book, page) == nil {
				return out, fmt.Errorf("wbxml: unknown code page %d", page)
			}
			d.current = page
		case tok == wbStrI:
			s, err := d.readString()
			if err != nil {
				return out, err
			}
			out.text += s
		case tok == wbStrT:
			s, err := d.readStringRef()
			if err != nil {
				return out, err
			}
			out.text += s
		case tok == wbEntity:
			v, err := d.readMBUint32()
			if err != nil {
				return out, err
			}
			out.text += string(rune(v))
		case tok == wbPI:
			n, err := d.readMBUint32()
			if err != nil {
				return out, err
			}
			if n > 1<<20 {
				return out, errors.New("wbxml: PI data too large")
			}
			if _, err := io.CopyN(io.Discard, d.r, int64(n)); err != nil {
				return out, err
			}
		case tok == wbExtI0 || tok == wbExtI1 || tok == wbExtI2:
			// Extension inline string: length-prefixed; EAS never uses it.
			if _, err := d.readString(); err != nil {
				return out, err
			}
		case tok == wbOpaque:
			n, err := d.readMBUint32()
			if err != nil {
				return out, err
			}
			if n > 1<<20 {
				return out, errors.New("wbxml: OPAQUE data too large")
			}
			if _, err := io.CopyN(io.Discard, d.r, int64(n)); err != nil {
				return out, err
			}
		default:
			if err := d.r.UnreadByte(); err != nil {
				return out, err
			}
			child, err := d.element()
			if err != nil {
				return out, err
			}
			out.children = append(out.children, child)
		}
	}
}

// attributes parses an attribute list (attr tokens + values) until END.
func (d *wbxmlDecoder) attributes() ([]Attr, error) {
	var out []Attr
	var cur *Attr
	for {
		tok, err := d.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if tok == wbEnd {
			return out, nil
		}
		if tok == wbStrI {
			s, err := d.readString()
			if err != nil {
				return nil, err
			}
			if cur == nil {
				return nil, errors.New("wbxml: attribute value without name")
			}
			cur.Value = s
			cur = nil
			continue
		}
		if tok == wbStrT {
			s, err := d.readStringRef()
			if err != nil {
				return nil, err
			}
			if cur == nil {
				return nil, errors.New("wbxml: attribute value without name")
			}
			cur.Value = s
			cur = nil
			continue
		}
		if tok >= 0x05 {
			out = append(out, Attr{Name: fmt.Sprintf("attr%d", len(out))})
			cur = &out[len(out)-1]
			continue
		}
		return nil, fmt.Errorf("wbxml: unexpected attribute token 0x%02x", tok)
	}
}

func (d *wbxmlDecoder) skipAttributes() error {
	_, err := d.attributes()
	return err
}

// readString reads a null-terminated inline string.
func (d *wbxmlDecoder) readString() (string, error) {
	var b bytes.Buffer
	for {
		c, err := d.r.ReadByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			return b.String(), nil
		}
		b.WriteByte(c)
		if b.Len() > 1<<20 {
			return "", errors.New("wbxml: string too long")
		}
	}
}

// readStringRef resolves a STR_T index into the string table.
func (d *wbxmlDecoder) readStringRef() (string, error) {
	idx, err := d.readMBUint32()
	if err != nil {
		return "", err
	}
	if idx >= uint32(len(d.table)) {
		return "", errors.New("wbxml: string table index out of range")
	}
	i := int(idx)
	end := bytes.IndexByte(d.table[i:], 0)
	if end < 0 {
		end = len(d.table) - i
	}
	return string(d.table[i : i+end]), nil
}
