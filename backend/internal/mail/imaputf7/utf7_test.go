package imaputf7

import "testing"

func TestFolderName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"INBOX", "INBOX"},
		{"&XfJT0ZAB-", "已发送"}, // classic mUTF-7 example (Sent in CJK)
		{"&-", "&"},           // escaped ampersand
		{"Sent&-", "Sent&"},   // mixed ASCII + escaped ampersand
		{"&ZeVnLIqe-", "日本語"}, // classic mUTF-7 example
		{"Trash", "Trash"},
		{"&&", "&&"}, // invalid (implicit shift) — falls back to raw
	}
	for _, c := range cases {
		if got := FolderName(c.in); got != c.want {
			t.Errorf("FolderName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
