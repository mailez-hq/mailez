package mail

import (
	"strings"
	"testing"
)

func TestSanitizeSignatureHTMLKeepsSafeStyling(t *testing.T) {
	in := `<p style="color: rgb(255, 0, 0); font-family: 'Microsoft YaHei', sans-serif; font-size: 14px">`
	out := SanitizeSignatureHTML(in)
	for _, want := range []string{"color: rgb(255, 0, 0)", "font-family", "font-size: 14px"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q to survive, got %s", want, out)
		}
	}
}

func TestSanitizeSignatureHTMLNormalisesSingleQuotedStyle(t *testing.T) {
	out := SanitizeSignatureHTML("<p style='color:red'>Hi</p>")
	if !strings.Contains(out, `style="color: red"`) {
		t.Fatalf("style attribute not normalised: %s", out)
	}
}

func TestSanitizeSignatureHTMLDropsDangerousCSS(t *testing.T) {
	cases := []string{
		`<p style="background: url(javascript:alert(1))">x</p>`,
		`<p style="width: expression(alert(1))">x</p>`,
		`<p style="background: url(https://evil.example/pixel)">x</p>`,
		`<p style="color: red; background: url(https://evil.example/p)">x</p>`,
		`<p style="behavior: url(#default#time2)">x</p>`,
		`<p style="color: red/*x*/">x</p>`,
	}
	for _, in := range cases {
		out := SanitizeSignatureHTML(in)
		lower := strings.ToLower(out)
		for _, banned := range []string{"url(", "expression", "javascript:", "/*"} {
			if strings.Contains(lower, banned) {
				t.Errorf("%q survived sanitising %q: %s", banned, in, out)
			}
		}
	}
	out := SanitizeSignatureHTML(`<p style="color: red; background: url(https://evil.example/p)">x</p>`)
	if !strings.Contains(out, "color: red") {
		t.Fatalf("safe declaration lost alongside an unsafe one: %s", out)
	}
}

func TestSanitizeSignatureHTMLDropsUnsafeProps(t *testing.T) {
	out := SanitizeSignatureHTML(`<p style="position: fixed; color: blue">x</p>`)
	if strings.Contains(out, "position") {
		t.Fatalf("disallowed property survived: %s", out)
	}
	if !strings.Contains(out, "color: blue") {
		t.Fatalf("allowed property dropped: %s", out)
	}
}

func TestSanitizeSignatureHTMLKeepsImagesAndTables(t *testing.T) {
	out := SanitizeSignatureHTML(`<table><tr><td bgcolor="#eee"><img src="https://x.example/logo.png" width="120"></td></tr></table>`)
	for _, want := range []string{"<table>", "<img", "logo.png", "width=\"120\"", "bgcolor"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "<script") {
		t.Fatalf("script survived: %s", out)
	}
}
