package oletools

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/richardlehane/mscfb"
)

// Version mirrors the oletools version the response format is compatible
// with (rspamd only uses it for debug output).
const Version = "0.60.1"

type macro struct {
	Code        string `json:"code"`
	OleStream   string `json:"ole_stream"`
	VBAFilename string `json:"vba_filename"`
}

type analysis struct {
	Type        string `json:"type"`
	Keyword     string `json:"keyword"`
	Description string `json:"description"`
}

type fileResult struct {
	Type        string     `json:"type"`
	File        string     `json:"file"`
	VBAFilename string     `json:"vba_filename"`
	OleStream   string     `json:"ole_stream"`
	Macros      []macro    `json:"macros"`
	Analysis    []analysis `json:"analysis"`
}

type metaInfo struct {
	Type       string `json:"type"`
	ScriptName string `json:"script_name"`
	Version    string `json:"version"`
	ReturnCode int    `json:"return_code"`
}

// Scan analyses an attachment and returns the olevba-compatible JSON array
// (MetaInformation first, then the per-file result when the file carries a
// VBA project). Non-office input returns an olefy-style error object.
func Scan(name string, data []byte) []byte {
	ok, macros, analysis := scanBytes(name, data)
	if !ok {
		return mustJSON([]any{
			map[string]any{"error": "Not an Office document"},
		})
	}
	if len(macros) == 0 {
		// olevba omits the file entry for documents without a VBA project;
		// rspamd treats that as "No macro found" and caches OK.
		return mustJSON([]any{metaInfo{Type: "MetaInformation", ScriptName: "olevba", Version: Version, ReturnCode: 0}})
	}
	return mustJSON([]any{
		metaInfo{Type: "MetaInformation", ScriptName: "olevba", Version: Version, ReturnCode: 0},
		fileResult{
			Type:        "file",
			File:        name,
			VBAFilename: name,
			OleStream:   "",
			Macros:      macros,
			Analysis:    analysis,
		},
	})
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`[{"error":"Unhandled error"}]`)
	}
	return b
}

// scanBytes returns ok=false for non-office files, otherwise the extracted
// macros and the mraptor-style analysis of their source code.
func scanBytes(name string, data []byte) (bool, []macro, []analysis) {
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		return scanCFB(name, data)
	}
	if isZip(data) {
		if vba := extractOOXMLVBA(data); len(vba) > 0 {
			return scanCFB(name+"/vbaProject.bin", vba)
		}
		return true, nil, nil
	}
	return false, nil, nil
}

func isZip(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && (data[2] == 3 || data[2] == 5) && data[3] == 4
}

// extractOOXMLVBA returns the first vbaProject.bin found in an OOXML package.
func extractOOXMLVBA(data []byte) []byte {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), "vbaproject.bin") {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close()
			b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
			if err != nil {
				return nil
			}
			return b
		}
	}
	return nil
}

// scanCFB parses an MS-CFB file and extracts VBA module streams.
func scanCFB(name string, data []byte) (bool, []macro, []analysis) {
	doc, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return true, nil, nil
	}
	hasVBAProject := false
	streams := map[string][]byte{}
	for {
		entry, err := doc.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return true, nil, nil
		}
		fullPath := append(append([]string{}, entry.Path...), entry.Name)
		joined := strings.Join(fullPath, "/")
		if entry.Name == "_VBA_PROJECT" || entry.Name == "PROJECT" {
			hasVBAProject = true
		}
		if entry.Size <= 0 || entry.Size > 64<<20 {
			continue
		}
		raw := make([]byte, entry.Size)
		if _, err := io.ReadFull(entry, raw); err != nil && err != io.ErrUnexpectedEOF {
			continue
		}
		streams[joined] = raw
	}
	if !hasVBAProject && !hasVBADir(streams) {
		return true, nil, nil
	}

	modules := moduleStreams(streams)
	sort.Strings(modules)
	var macros []macro
	var allAnalysis []analysis
	for _, name := range modules {
		code := DecodeModule(streams[name])
		if code == "" {
			continue
		}
		macros = append(macros, macro{
			Code:        code,
			OleStream:   name,
			VBAFilename: path.Base(name),
		})
		allAnalysis = append(allAnalysis, analyzeCode(code)...)
	}
	return true, macros, allAnalysis
}

func hasVBADir(streams map[string][]byte) bool {
	for name := range streams {
		if strings.HasPrefix(name, "VBA/") || strings.HasPrefix(name, "Macros/") {
			return true
		}
	}
	return false
}

// moduleStreams lists VBA module streams, excluding the project bookkeeping
// streams (dir / PROJECT / _VBA_PROJECT / PROJECTwm).
func moduleStreams(streams map[string][]byte) []string {
	var out []string
	for name := range streams {
		base := path.Base(name)
		switch base {
		case "dir", "PROJECT", "_VBA_PROJECT", "PROJECTwm":
			continue
		}
		if strings.HasPrefix(name, "VBA/") || strings.HasPrefix(name, "Macros/") {
			out = append(out, name)
		}
	}
	return out
}

var (
	autoexecRe = regexp.MustCompile(`(?i)\b(?:Auto(?:Exec|_?Open|_?Close|Exit|New)|Document(?:_?Open|_Close|_?BeforeClose|Change|_New)|NewDocument|Workbook(?:_Open|_Activate|_Close|_BeforeClose)|\w+_(?:Painted|Painting|GotFocus|LostFocus|MouseHover|Layout|Click|Change|Resize|BeforeNavigate2|BeforeScriptExecute|DocumentComplete|DownloadBegin|DownloadComplete|FileDownload|NavigateComplete2|NavigateError|ProgressChange|PropertyChange|SetSecureLockIcon|StatusTextChange|TitleChange|MouseMove|MouseEnter|MouseLeave|OnConnecting))\b|Auto_Ope\b`)
	fileRe     = regexp.MustCompile(`(?i)\b(?:FileCopy|CopyFile|Kill|CreateTextFile|VirtualAlloc|RtlMoveMemory|URLDownloadToFileA?|AltStartupPath|WriteProcessMemory|ADODB\.Stream|WriteText|SaveToFile|SaveAs|SaveAsRTF|FileSaveAs|MkDir|RmDir|SaveSetting|SetAttr)\b|(?:\bOpen\b[^\n]+\b(?:Write|Append|Binary|Output|Random)\b)`)
	execRe     = regexp.MustCompile(`(?i)\b(?:Shell|CreateObject|GetObject|SendKeys|RUN|CALL|MacScript|FollowHyperlink|CreateThread|ShellExecuteA?|ExecuteExcel4Macro|EXEC|REGISTER|SetTimer)\b|(?:\bDeclare\b[^\n]+\bLib\b)`)
	urlRe      = regexp.MustCompile(`(?i)(?:https?|ftp)://[^\s"'<>]+`)
	ipRe       = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
)

// analyzeCode runs the same mraptor-style heuristics rspamd's patterns expect
// (AutoExec / Suspicious keywords, IOC URLs and IPs).
func analyzeCode(code string) []analysis {
	var out []analysis
	seen := map[string]bool{}
	add := func(kind, keyword, desc string) {
		if keyword == "" || seen[kind+"|"+keyword] {
			return
		}
		seen[kind+"|"+keyword] = true
		out = append(out, analysis{Type: kind, Keyword: keyword, Description: desc})
	}
	for _, m := range autoexecRe.FindAllString(code, -1) {
		add("AutoExec", m, "Runs automatically when the document is opened")
	}
	for _, m := range fileRe.FindAllString(code, -1) {
		add("Suspicious", m, "Suspicious file system operation")
	}
	for _, m := range execRe.FindAllString(code, -1) {
		add("Suspicious", m, "Suspicious process execution")
	}
	for _, m := range urlRe.FindAllString(code, -1) {
		add("IOC", m, "URL string")
	}
	for _, m := range ipRe.FindAllString(code, -1) {
		if ip := net.ParseIP(m); ip != nil {
			add("IOC", m, "IP address string")
		}
	}
	return out
}

// ErrorTooSmall is the olefy error for files below the scan threshold.
func ErrorTooSmall() []byte {
	return mustJSON([]any{map[string]any{"error": "File too small"}})
}

// ErrorProtocol mirrors olefy's protocol error response.
func ErrorProtocol() []byte {
	return mustJSON([]any{map[string]any{"error": "Protocol error"}})
}

// ErrorMethod mirrors olefy's missing-method response.
func ErrorMethod() []byte {
	return mustJSON([]any{map[string]any{"error": "Protocol error: Method header not found"}})
}
