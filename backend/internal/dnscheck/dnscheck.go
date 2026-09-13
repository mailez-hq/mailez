// Package dnscheck provides small, timeout-bounded DNS probes shared by the
// admin health center and the domain DNS wizard. Lookups distinguish a
// confirmed-absent record (NotFound) from a failed query (resolver
// unreachable, SERVFAIL, policy refusal) so callers render "missing" versus
// "unknown" instead of guessing.
package dnscheck

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

const timeout = 4 * time.Second

var resolver = &net.Resolver{}

// ErrNotFound is returned when the resolver authoritatively reports that the
// name has no such record. Anything else (network error, SERVFAIL, refused)
// is a failed probe, not an absent record.
var ErrNotFound = errors.New("record not found")

// IsNotFound reports whether err represents a confirmed absent record.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// TXT returns the TXT records for name.
func TXT(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	vals, err := resolver.LookupTXT(ctx, name)
	if err != nil {
		return nil, classify(err)
	}
	return vals, nil
}

// MX returns the MX host names for name with trailing dots trimmed.
func MX(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	mxs, err := resolver.LookupMX(ctx, name)
	if err != nil {
		return nil, classify(err)
	}
	out := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		out = append(out, strings.TrimSuffix(mx.Host, "."))
	}
	return out, nil
}

// CNAME returns the canonical target of name (trailing dot trimmed).
func CNAME(ctx context.Context, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	target, err := resolver.LookupCNAME(ctx, name)
	if err != nil {
		return "", classify(err)
	}
	return strings.TrimSuffix(target, "."), nil
}

// Hosts returns the A/AAAA addresses for name.
func Hosts(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addrs, err := resolver.LookupHost(ctx, name)
	if err != nil {
		return nil, classify(err)
	}
	return addrs, nil
}

// NormalizeTXT collapses DNS label splitting, whitespace and case so two
// renderings of the same record compare equal. Case-insensitive comparison
// is safe for record *policies* (v=SPF1 / v=DKIM1 / key tags); the base64
// key material must be compared through PValue instead.
func NormalizeTXT(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '\t', '"', '\r', '\n':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// PValue extracts the p= tag payload (base64 key) from a DKIM TXT record,
// whitespace-stripped. Returns "" when absent.
func PValue(txt string) string {
	for _, part := range strings.Split(txt, ";") {
		part = strings.TrimSpace(part)
		if len(part) >= 2 && strings.EqualFold(part[:2], "p=") {
			return strings.ReplaceAll(part[2:], " ", "")
		}
	}
	return ""
}

// HasRecordFold reports whether any of records contains sub
// case-insensitively.
func HasRecordFold(records []string, sub string) bool {
	for _, r := range records {
		if strings.Contains(strings.ToLower(r), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// classify converts net errors into ErrNotFound when the resolver positively
// reported NXDOMAIN/no records, and leaves other errors as-is.
func classify(err error) error {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return ErrNotFound
	}
	return err
}
