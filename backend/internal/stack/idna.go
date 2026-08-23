package stack

import (
	"strings"

	"golang.org/x/net/idna"
)

// asciiDomain lowercases and IDNA-encodes a domain for the wire contract.
// the the mail stack stores and emits punycode, so the mail stack (postfix/rspamd) only
// sees ASCII even for Unicode domains.
func asciiDomain(domain string) string {
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil || ascii == "" {
		return domain
	}
	return ascii
}

// domainCandidates returns lookup keys covering both storage conventions
// (ASCII punycode and Unicode), deduplicated.
func domainCandidates(domain string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	add(domain)
	add(asciiDomain(domain))
	if u, err := idna.Lookup.ToUnicode(domain); err == nil {
		add(u)
	}
	return out
}

// asciiEmail IDNA-encodes the domain part of an email address.
func asciiEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return asciiDomain(email)
	}
	return email[:at] + "@" + asciiDomain(email[at+1:])
}
