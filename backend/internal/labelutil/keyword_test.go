package labelutil

import "testing"

func TestEncodeKeyword(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Work", "Work"},          // pure ASCII passes through
		{"工作", "=E5=B7=A5=E4=BD=9C"}, // Chinese is =XX-encoded per UTF-8 byte
		{"a b", "a=20b"},          // space (atom-special) encoded
		{"a(b)c", "a=28b=29c"},    // atom-specials encoded
		{"100%", "100=25"},        // list-wildcard encoded
		{"", ""},                  // empty stays empty
	}
	for _, tc := range cases {
		if got := EncodeKeyword(tc.name); got != tc.want {
			t.Errorf("EncodeKeyword(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEncodeKeywordAtomSafe(t *testing.T) {
	// Every encoded output must consist only of IMAP atom characters and
	// stay within the 64-octet keyword limit.
	for _, name := range []string{"工作", "a b", "a(b)", "x%y", `a"b`, "a\\b", "a]b"} {
		kw := EncodeKeyword(name)
		if len(kw) > MaxKeywordBytes {
			t.Errorf("EncodeKeyword(%q) = %q exceeds %d bytes", name, kw, MaxKeywordBytes)
		}
		for i := 0; i < len(kw); i++ {
			if !atomSafe(rune(kw[i])) {
				t.Errorf("EncodeKeyword(%q) = %q contains atom-unsafe byte %q", name, kw, kw[i])
			}
		}
	}
}

func TestCanonicalFlag(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Bare display names (what API clients post) resolve to system flags.
		{"seen", `\Seen`},
		{"flagged", `\Flagged`},
		{"answered", `\Answered`},
		{"deleted", `\Deleted`},
		{"draft", `\Draft`},
		// Already-canonical and case variants pass through normalized.
		{`\Seen`, `\Seen`},
		{`\seen`, `\Seen`},
		{"FLAGGED", `\Flagged`},
		{"\\Flagged", `\Flagged`},
		// Keywords and encoded label names are untouched.
		{"$MDNSent", "$MDNSent"},
		{"$Snoozed", "$Snoozed"},
		{"工作", "工作"},
		{"Work", "Work"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := CanonicalFlag(tc.in); got != tc.want {
			t.Errorf("CanonicalFlag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
