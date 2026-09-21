package mail

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DisconnectSessions asks the engine's management API to drop an account's
// live sessions; a no-op without a configured management endpoint. Every
// resolved address is called: the engine service name is a round-robin of
// replicas that each keep their own session table, so one call would only
// reach the sessions that happen to live on one of them.
func DisconnectSessions(addr, secret, email string) error {
	if addr == "" {
		return nil
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	base, err := url.Parse(strings.TrimRight(addr, "/"))
	if err != nil {
		return err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/accounts/" + url.PathEscape(email) + "/disconnect"

	hosts := []string{base.Host}
	if host, port, err := net.SplitHostPort(base.Host); err == nil && net.ParseIP(host) == nil {
		if ips, err := net.LookupHost(host); err == nil && len(ips) > 0 {
			hosts = hosts[:0]
			for _, ip := range ips {
				hosts = append(hosts, net.JoinHostPort(ip, port))
			}
		}
	}

	var firstErr error
	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, host := range hosts {
		target := *base
		target.Host = host
		req, err := http.NewRequest(http.MethodPost, target.String(), nil)
		if err != nil {
			fail(err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			fail(err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fail(fmt.Errorf("disconnect %s on %s: engine status %d", email, host, resp.StatusCode))
		}
	}
	return firstErr
}
