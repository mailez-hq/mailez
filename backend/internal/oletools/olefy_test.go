package oletools

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"unicode/utf16"
)

// buildTestCFB constructs a minimal MS-CFB file. With vba=true it carries a
// VBA project (PROJECT + _VBA_PROJECT + VBA/dir + VBA/ThisDocument); streams
// are stored in the mini stream.
func buildTestCFB(vba bool, thisDoc string) []byte {
	doc := utf16Bytes(thisDoc)
	project := []byte("ID={00000000-0000-0000-0000-000000000000}\r\n")
	vbaProject := []byte{1, 0, 0, 0}
	dir := []byte("Dummy dir stream")

	// Mini stream layout: PROJECT(1), _VBA_PROJECT(1), dir(1), ThisDocument(n)
	miniSectors := 64
	pad := func(b []byte) []byte {
		for len(b)%miniSectors != 0 {
			b = append(b, 0)
		}
		return b
	}
	var miniStream []byte
	if vba {
		miniStream = bytes.Join([][]byte{
			pad(project), pad(vbaProject), pad(dir), pad(doc),
		}, nil)
	} else {
		miniStream = pad(doc)
	}
	numMini := len(miniStream) / miniSectors
	docStart := 0
	dirStart := 0
	vbaProjectStart := 0
	projectStart := 0
	rootChild := uint32(0xFFFFFFFF)
	if vba {
		docStart = 3
		dirStart = 2
		vbaProjectStart = 1
		projectStart = 0
		rootChild = 1
	}

	// Mini FAT chains: PROJECT 0, _VBA_PROJECT 1, dir 2, ThisDocument 3..n-1
	miniFAT := make([]uint32, 128)
	for i := range miniFAT {
		miniFAT[i] = 0xFFFFFFFF
	}
	for i := 0; i < numMini-1; i++ {
		miniFAT[i] = uint32(i + 1)
	}
	if numMini > 0 {
		miniFAT[numMini-1] = 0xFFFFFFFE
	}

	// Sector layout (mscfb numbers the sector after the header as 0):
	// 0 FAT, 1-2 directory, 3 mini FAT, 4 mini stream
	const sector = 512
	numDirSectors := 2
	fat := make([]uint32, sector/4)
	for i := range fat {
		fat[i] = 0xFFFFFFFF
	}
	fat[0] = 0xFFFFFFFD // FAT sector
	fat[1] = 2          // directory chain
	fat[2] = 0xFFFFFFFE
	fat[3] = 0xFFFFFFFE // mini FAT
	fat[4] = 0xFFFFFFFE // mini stream

	// Directory entries (order: Root, VBA, dir, ThisDocument, PROJECT, _VBA_PROJECT)
	numEntries := 6
	if !vba {
		numEntries = 2
	}
	entries := make([]byte, numEntries*128)
	writeDirEntry := func(idx int, name string, typ byte, left, right, child uint32, start uint32, size uint32) {
		off := idx * 128
		u := utf16Bytes(name)
		u = append(u, 0, 0) // NUL terminator
		copy(entries[off:], u)
		binary.LittleEndian.PutUint16(entries[off+64:], uint16(len(u)))
		entries[off+66] = typ
		entries[off+67] = 1 // black
		binary.LittleEndian.PutUint32(entries[off+68:], left)
		binary.LittleEndian.PutUint32(entries[off+72:], right)
		binary.LittleEndian.PutUint32(entries[off+76:], child)
		binary.LittleEndian.PutUint32(entries[off+116:], start)
		binary.LittleEndian.PutUint32(entries[off+120:], size)
	}
	// Root Entry -> child VBA(1); VBA -> child dir(2); dir -> right ThisDocument(3);
	// VBA(1) right -> PROJECT(4); PROJECT right -> _VBA_PROJECT(5)
	writeDirEntry(0, "Root Entry", 5, 0xFFFFFFFF, 0xFFFFFFFF, rootChild, 4, uint32(len(miniStream)))
	if vba {
		writeDirEntry(1, "VBA", 1, 0xFFFFFFFF, 4, 2, 0, 0)
		writeDirEntry(2, "dir", 2, 0xFFFFFFFF, 3, 0xFFFFFFFF, uint32(dirStart), uint32(len(dir)))
		writeDirEntry(3, "ThisDocument", 2, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, uint32(docStart), uint32(len(doc)))
		writeDirEntry(4, "PROJECT", 2, 0xFFFFFFFF, 5, 0xFFFFFFFF, uint32(projectStart), uint32(len(project)))
		writeDirEntry(5, "_VBA_PROJECT", 2, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, uint32(vbaProjectStart), uint32(len(vbaProject)))
	} else {
		writeDirEntry(1, "WordDocument", 2, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, uint32(docStart), uint32(len(doc)))
	}

	dirSectors := make([]byte, numDirSectors*sector)
	copy(dirSectors, entries)
	fatBytes := make([]byte, sector)
	for i, v := range fat {
		binary.LittleEndian.PutUint32(fatBytes[i*4:], v)
	}
	miniFATBytes := make([]byte, sector)
	for i, v := range miniFAT {
		binary.LittleEndian.PutUint32(miniFATBytes[i*4:], v)
	}
	miniStreamBytes := make([]byte, sector)
	copy(miniStreamBytes, miniStream)

	header := make([]byte, sector)
	copy(header, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(header[0x1A:], 0x0003)     // major version
	binary.LittleEndian.PutUint16(header[0x1C:], 0xFFFE)     // byte order
	binary.LittleEndian.PutUint16(header[0x1E:], 0x0009)     // sector shift 512
	binary.LittleEndian.PutUint16(header[0x20:], 0x0006)     // mini sector shift 64
	binary.LittleEndian.PutUint32(header[0x2C:], 1)          // num FAT sectors
	binary.LittleEndian.PutUint32(header[0x28:], 2)          // num directory sectors
	binary.LittleEndian.PutUint32(header[0x30:], 1)          // first directory sector
	binary.LittleEndian.PutUint32(header[0x38:], 4096)       // mini stream cutoff
	binary.LittleEndian.PutUint32(header[0x3C:], 3)          // first mini FAT sector
	binary.LittleEndian.PutUint32(header[0x40:], 1)          // num mini FAT sectors
	binary.LittleEndian.PutUint32(header[0x44:], 0xFFFFFFFE) // no DIFAT sectors
	binary.LittleEndian.PutUint32(header[0x4C:], 0)          // DIFAT[0] -> FAT sector 0

	out := bytes.Join([][]byte{header, fatBytes, dirSectors, miniFATBytes, miniStreamBytes}, nil)
	return out
}

func utf16Bytes(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

func TestScanCFBWithMacros(t *testing.T) {
	code := "Sub AutoOpen()\r\n  Shell \"cmd.exe /c calc\"\r\n  CreateObject(\"WScript.Shell\")\r\nEnd Sub\r\n"
	cfb := buildTestCFB(true, code)
	if len(cfb) == 0 {
		t.Fatal("empty cfb")
	}
	resp := Scan("test.doc", cfb)
	if !strings.Contains(string(resp), `"macros"`) {
		t.Fatalf("expected macros in response: %s", resp)
	}
	var items []map[string]any
	if err := json.Unmarshal(resp, &items); err != nil {
		t.Fatalf("response not json: %v (%s)", err, resp)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %s", len(items), resp)
	}
	if items[0]["type"] != "MetaInformation" {
		t.Fatalf("first item not MetaInformation: %s", resp)
	}
	file := items[1]
	macros, _ := file["macros"].([]any)
	if len(macros) != 1 {
		t.Fatalf("expected 1 macro, got %d: %s", len(macros), resp)
	}
	analysis, _ := file["analysis"].([]any)
	types := map[string]bool{}
	for _, a := range analysis {
		m := a.(map[string]any)
		types[m["type"].(string)] = true
	}
	if !types["AutoExec"] || !types["Suspicious"] {
		t.Fatalf("missing analysis types: %v in %s", types, resp)
	}
}

func TestScanCleanDocument(t *testing.T) {
	cfb := buildTestCFB(false, "Sub NoMacro()\r\nEnd Sub\r\n")
	resp := Scan("clean.doc", cfb)
	var items []map[string]any
	if err := json.Unmarshal(resp, &items); err != nil {
		t.Fatalf("not json: %v", err)
	}
	if len(items) != 1 || items[0]["type"] != "MetaInformation" {
		t.Fatalf("clean doc should have no file entry: %s", resp)
	}
}

func TestScanNonOffice(t *testing.T) {
	resp := Scan("notes.txt", []byte("plain text"))
	if !strings.Contains(string(resp), `"error"`) {
		t.Fatalf("expected error json: %s", resp)
	}
}

func TestDecompressLiterals(t *testing.T) {
	// signature 0x01; chunk header 0xB009 (9 data bytes, compressed, sig 011);
	// flag byte 0x00 + 8 literal bytes.
	container := []byte{0x01, 0x09, 0xB0, 0x00, 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'}
	out, err := Decompress(container)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if string(out) != "ABCDEFGH" {
		t.Fatalf("decompressed wrong: %q", out)
	}
}

func TestDecompressCopyToken(t *testing.T) {
	// 4 literals "ABCD" then a copy token (offset 4, length 4) duplicating them.
	// chunk data = flag(1) + literals(4) + copy token(2) = 7 -> field 7.
	// flag byte 0x10: first 4 tokens literal, 5th a copy token.
	container := []byte{0x01, 0x07, 0xB0, 0x10, 'A', 'B', 'C', 'D', 0x01, 0x30}
	out, err := Decompress(container)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if string(out) != "ABCDABCD" {
		t.Fatalf("decompressed wrong: %q", out)
	}
}

func TestDecodeModuleUTF16(t *testing.T) {
	raw := utf16Bytes("Sub Test()\r\nEnd Sub\r\n")
	if got := DecodeModule(raw); !strings.Contains(got, "Sub Test()") {
		t.Fatalf("decode wrong: %q", got)
	}
}

func TestScanOOXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("word/vbaProject.bin")
	f.Write(buildTestCFB(true, "Sub AutoExec()\r\nShell \"calc.exe\"\r\nEnd Sub\r\n"))
	zw.Close()
	resp := Scan("evil.docm", buf.Bytes())
	if !strings.Contains(string(resp), `"AutoExec"`) {
		t.Fatalf("expected autoexec analysis in ooxml response: %s", resp)
	}
}

func TestOlefyProtocol(t *testing.T) {
	srv := &Server{MinLength: 10}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(conn)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("PING"))
	conn.(*net.TCPConn).CloseWrite()
	buf := make([]byte, 4)
	if _, err := conn.Read(buf); err != nil || string(buf) != "PONG" {
		t.Fatalf("ping: %q %v", buf, err)
	}
	conn.Close()

	conn2, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	conn2.Write([]byte("OLEFY/1.0\nMethod: oletools\nRspamd-ID: abc\n\nplain text"))
	conn2.(*net.TCPConn).CloseWrite()
	resp := make([]byte, 4096)
	n, _ := conn2.Read(resp)
	if !strings.Contains(string(resp[:n]), `"error"`) {
		t.Fatalf("expected error for non-office payload: %s", resp[:n])
	}
}
