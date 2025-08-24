package mail

import "testing"

func TestInboxPathAndNameCanonicalizePrefix(t *testing.T) {
	cases := []struct {
		in   string
		path string // display spelling used by ListFolders / the sidebar
		name string // IMAP spelling used by every mailbox operation
	}{
		{"INBOX", "Inbox", "INBOX"},
		{"Inbox", "Inbox", "INBOX"},
		{"inbox", "Inbox", "INBOX"},
		{"INBOX/Projects", "Inbox/Projects", "INBOX/Projects"},
		{"inbox/Projects/Invoice", "Inbox/Projects/Invoice", "INBOX/Projects/Invoice"},
		{"Projects/Invoice", "Projects/Invoice", "Projects/Invoice"},
		{"Sent", "Sent", "Sent"},
		{"Inbox-Other", "Inbox-Other", "Inbox-Other"},
	}
	for _, c := range cases {
		if got := inboxPath(c.in); got != c.path {
			t.Errorf("inboxPath(%q) = %q, want %q", c.in, got, c.path)
		}
		if got := inboxName(c.in); got != c.name {
			t.Errorf("inboxName(%q) = %q, want %q", c.in, got, c.name)
		}
	}
}
