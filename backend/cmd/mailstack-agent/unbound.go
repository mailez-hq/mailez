package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"mailez/backend/internal/agent"
)

const unboundConfPath = "/etc/unbound/unbound.conf"

// unboundConfig mirrors the variables consumed by the legacy Jinja template
// (deploy/vendor/mailstack/unbound/unbound.conf).
type unboundConfig struct {
	subnet  string
	subnet6 string
	ipv6    bool
}

func loadUnboundConfig() (unboundConfig, error) {
	subnet, err := agent.CIDR("SUBNET")
	if err != nil {
		return unboundConfig{}, err
	}
	cfg := unboundConfig{subnet: subnet}
	if v := os.Getenv("SUBNET6"); v != "" {
		if _, _, err := net.ParseCIDR(v); err != nil {
			return unboundConfig{}, fmt.Errorf("SUBNET6=%q is not a valid CIDR: %w", v, err)
		}
		cfg.subnet6 = v
		cfg.ipv6 = true
	}
	return cfg, nil
}

// render produces the unbound.conf content. Semantics match the vendored
// Jinja template, with one deliberate fix: the template's whitespace control
// (`{{-`) collapses the IPv6 interface line onto the IPv4 one when SUBNET6 is
// set, producing an invalid config; we always emit them on separate lines.
func (c unboundConfig) render() []byte {
	var b bytes.Buffer
	b.WriteString("server:\n")
	b.WriteString("  verbosity: 1\n")
	b.WriteString("  interface: 0.0.0.0\n")
	if c.ipv6 {
		b.WriteString("  interface: ::0\n")
	}
	b.WriteString("  logfile: \"\"\n")
	b.WriteString("  do-ip4: yes\n")
	b.WriteString(fmt.Sprintf("  do-ip6: %s\n", yesNo(c.ipv6)))
	b.WriteString("  do-udp: yes\n")
	b.WriteString("  do-tcp: yes\n")
	b.WriteString("  do-daemonize: no\n")
	b.WriteString(fmt.Sprintf("  access-control: %s allow\n", c.subnet))
	if c.ipv6 {
		b.WriteString(fmt.Sprintf("  access-control: %s allow\n", c.subnet6))
	}
	b.WriteString("  directory: \"/etc/unbound\"\n")
	b.WriteString("  username: unbound\n")
	b.WriteString("  auto-trust-anchor-file: trusted-key.key\n")
	b.WriteString("  root-hints: \"/etc/unbound/root.hints\"\n")
	b.WriteString("  hide-identity: yes\n")
	b.WriteString("  hide-version: yes\n")
	b.WriteString("  cache-min-ttl: 300\n")
	return b.Bytes()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func runUnbound() error {
	cfg, err := loadUnboundConfig()
	if err != nil {
		return err
	}
	if err := agent.AtomicWrite(unboundConfPath, cfg.render(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", unboundConfPath, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return agent.RunChild(ctx, []string{"/usr/sbin/unbound", "-c", unboundConfPath})
}
