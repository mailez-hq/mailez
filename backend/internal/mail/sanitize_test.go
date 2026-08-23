package mail

import (
	"strings"
	"testing"
)

func TestSanitizeHTMLStripsExecutableContent(t *testing.T) {
	in := `<p>safe <b>bold</b></p><script>alert(1)</script><img src="x" onerror="alert(2)"><a href="https://example.com" onclick="x()">link</a><iframe src="https://evil.test"></iframe><form action="https://evil.test"><input></form><table><tr><td>cell</td></tr></table>`
	out := SanitizeHTML(in)
	for _, banned := range []string{"<script", "onerror", "onclick", "<iframe", "<form", "<input"} {
		if strings.Contains(out, banned) {
			t.Errorf("sanitized output still contains %q:\n%s", banned, out)
		}
	}
	for _, kept := range []string{"<b>bold</b>", "https://example.com", "<table>", "<td>cell</td>"} {
		if !strings.Contains(out, kept) {
			t.Errorf("sanitized output lost %q:\n%s", kept, out)
		}
	}
}
