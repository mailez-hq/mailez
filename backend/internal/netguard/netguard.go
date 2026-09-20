// Package netguard serves URLs the backend fetches for users: webhooks,
// one-click unsubscribes, CardDAV endpoints. Those URLs are an SSRF surface.
// Validate rejects the obvious cases at save time; NewClient re-checks the
// resolved IP at dial time, so a public hostname pointing at a private address
// cannot slip through either.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlocked is returned for a resolved address the guard refuses.
var ErrBlocked = errors.New("target address is not publicly routable")

// RFC 6598 CGNAT space.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func blocked(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() ||
		cgnat.Contains(ip)
}

// Validate is the save-time check on a user-supplied URL: http(s) only, no
// localhost spellings, no literal private or reserved IPs. Hostnames are not
// resolved here; NewClient does that at dial time.
func Validate(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return errors.New("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url must be http(s)")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url must include a host")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || lower == "metadata.google.internal" {
		return errors.New("local addresses are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && blocked(ip) {
		return ErrBlocked
	}
	return nil
}

// NewClient builds an HTTP client for user-supplied URLs. It refuses a
// connection whose resolved IP is loopback, private, link-local or CGNAT, and
// returns redirects to the caller instead of following them: a redirect target
// could point at an internal address.
func NewClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("netguard: bad dial address %q", address)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("netguard: dial target %q is not an IP", host)
			}
			if blocked(ip) {
				return fmt.Errorf("netguard: %w", ErrBlocked)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
