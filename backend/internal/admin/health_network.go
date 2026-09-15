// Network-facing health checks. The domain probes answer "can mail arrive
// here"; these answer "can mail leave, and does the network vouch for this
// host": outbound relay reachability, reverse DNS agreement and the process
// resolver's identity.
package admin

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"mailez/backend/internal/dnscheck"
)

// outboundProbeHostDefault is dialed on port 25 when no probe host is
// configured. The point is whether the local network lets port 25 out at
// all, not reachability of one provider, so any large anycast MTA works.
const outboundProbeHostDefault = "aspmx.l.google.com"

// publicResolvers are resolver addresses operated as global public
// services. Reputation DNSBLs (Spamhaus in particular) refuse queries
// arriving from them, so a deployment resolving through one loses blacklist
// screening and health checks degrade to unknown where listings should be.
var publicResolvers = map[string]bool{
	"8.8.8.8": true, "8.8.4.4": true,
	"2001:4860:4860::8888": true, "2001:4860:4860::8844": true,
	"1.1.1.1": true, "1.0.0.1": true,
	"2606:4700:4700::1111": true, "2606:4700:4700::1001": true,
	"9.9.9.9": true, "149.112.112.112": true,
	"2620:fe::fe": true, "2620:fe::9": true,
	"208.67.222.222": true, "208.67.220.220": true,
	"223.5.5.5": true, "223.6.6.6": true,
	"119.29.29.29": true, "114.114.114.114": true,
}

// networkChecks runs the network-facing probes. PTR needs a real hostname;
// localhost deployments (development) skip it.
func (h *Handler) networkChecks(ctx context.Context) []CheckItem {
	items := []CheckItem{checkOutbound25(h.Cfg.OutboundProbeHost)}
	if item, have := resolverCheck(); have {
		items = append(items, item)
	}
	if host := h.Cfg.Hostname; host != "" && host != "localhost" && host != "127.0.0.1" {
		items = append(items, ptrCheck(ctx, host))
	}
	return items
}

// checkOutbound25 dials the probe host on port 25. Cloud and residential
// providers commonly block outbound 25, which silently stops all sending
// while every other check stays green.
func checkOutbound25(probeHost string) CheckItem {
	host := probeHost
	if host == "" {
		host = outboundProbeHostDefault
	}
	addr := net.JoinHostPort(host, "25")
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err == nil {
		_ = conn.Close()
		return ok("outbound25", addr)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		// The probe host itself did not resolve: a resolver problem on
		// this network, not a blocked port.
		return unknown("outbound25", addr+" probe failed: "+err.Error())
	}
	return fail("outbound25", addr+" unreachable ("+err.Error()+") — outbound mail is blocked; relay through a smarthost or ask the provider to unblock port 25")
}

// ptrCheck verifies that reverse DNS of the mail host's addresses agrees
// with the hostname used in EHLO. Receivers weight this heavily: a missing
// PTR fails, and a name pointing elsewhere warns — relay setups do that on
// purpose.
func ptrCheck(ctx context.Context, hostname string) CheckItem {
	addrs, err := dnscheck.Hosts(ctx, hostname)
	if err != nil || len(addrs) == 0 {
		return unknown("ptr", "cannot resolve "+hostname)
	}
	var noPTR, mismatch, match []string
	for _, a := range addrs {
		names, err := dnscheck.Reverse(ctx, a)
		switch {
		case dnscheck.IsNotFound(err):
			noPTR = append(noPTR, a)
			continue
		case err != nil:
			return unknown("ptr", a+": "+err.Error())
		}
		if dnscheck.HasRecordFold(names, hostname) {
			match = append(match, a)
		} else {
			mismatch = append(mismatch, a+" → "+strings.Join(names, ","))
		}
	}
	if len(noPTR) > 0 {
		return fail("ptr", strings.Join(noPTR, ", ")+" has no PTR record")
	}
	if len(mismatch) > 0 {
		return warn("ptr", strings.Join(mismatch, "; ")+" does not match "+hostname)
	}
	return ok("ptr", strings.Join(match, ", "))
}

// resolverCheck grades the resolver this process uses via resolv.conf.
// It returns have=false outside Linux, where the file does not exist
// (developer machines).
func resolverCheck() (item CheckItem, have bool) {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return CheckItem{}, false
	}
	ns, public := parseResolvConf(string(data))
	if len(ns) == 0 {
		return CheckItem{}, false
	}
	if len(public) > 0 {
		return warn("resolver", strings.Join(public, ", ")+" is a public resolver; reputation DNSBLs refuse its queries"), true
	}
	return ok("resolver", strings.Join(ns, ", ")), true
}

// parseResolvConf extracts the nameserver lines and classifies them.
func parseResolvConf(data string) (ns, public []string) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		ns = append(ns, fields[1])
		if publicResolvers[strings.ToLower(fields[1])] {
			public = append(public, fields[1])
		}
	}
	return ns, public
}
