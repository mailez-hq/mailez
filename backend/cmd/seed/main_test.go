package main

import "testing"

func TestLocalpartFor(t *testing.T) {
	for _, c := range []struct{ email, domain, want string }{
		{"admin@example.com", "example.com", "admin"},
		{"admin@mail.mailez.co.jp", "mail.mailez.co.jp", "admin"},
		{"Admin@Example.com", "example.com", "Admin"},
	} {
		got, err := localpartFor(c.email, c.domain)
		if err != nil || got != c.want {
			t.Errorf("localpartFor(%q, %q) = %q, %v; want %q", c.email, c.domain, got, err, c.want)
		}
	}

	// A 17-char domain with the default admin@example.com is the report that
	// reached us: same length, so the old slicing panicked with [:-1].
	for _, c := range []struct{ email, domain string }{
		{"admin@example.com", "mail.mailez.co.jp"},
		{"admin", "example.com"},
		{"@example.com", "example.com"},
	} {
		if _, err := localpartFor(c.email, c.domain); err == nil {
			t.Errorf("localpartFor(%q, %q) accepted a bad pair", c.email, c.domain)
		}
	}
}
