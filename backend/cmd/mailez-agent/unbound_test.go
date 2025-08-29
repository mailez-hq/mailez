//go:build mailez_ee

package main

import (
	"strings"
	"testing"
)

func TestUnboundRenderIPv4Only(t *testing.T) {
	cfg := unboundConfig{subnet: "192.168.206.0/24"}
	want := `server:
  verbosity: 1
  interface: 0.0.0.0
  logfile: ""
  do-ip4: yes
  do-ip6: no
  do-udp: yes
  do-tcp: yes
  do-daemonize: no
  access-control: 192.168.206.0/24 allow
  directory: "/etc/unbound"
  username: unbound
  auto-trust-anchor-file: trusted-key.key
  root-hints: "/etc/unbound/root.hints"
  hide-identity: yes
  hide-version: yes
  cache-min-ttl: 300
`
	if got := string(cfg.render()); got != want {
		t.Fatalf("render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestUnboundRenderIPv6(t *testing.T) {
	cfg := unboundConfig{subnet: "192.168.206.0/24", subnet6: "fd00::/64", ipv6: true}
	got := string(cfg.render())
	for _, line := range []string{
		"  interface: 0.0.0.0\n",
		"  interface: ::0\n",
		"  do-ip6: yes\n",
		"  access-control: fd00::/64 allow\n",
	} {
		if !strings.Contains(got, line) {
			t.Fatalf("render missing %q in:\n%s", line, got)
		}
	}
	if strings.Contains(got, "0.0.0.0  interface") {
		t.Fatalf("render collapsed interface lines:\n%s", got)
	}
}
