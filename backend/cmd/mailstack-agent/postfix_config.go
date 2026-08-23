package main

import (
	"fmt"
	"os"
	"strings"

	"mailez/backend/internal/agent"
)

// PostfixConfig is the typed, validated view of the environment consumed by
// the postfix templates. It mirrors the Jinja variables used by the vendored
// main.cf/master.cf/mta-sts-daemon.yml.
type PostfixConfig struct {
	Domain                  string
	Hostname                string
	MessageSizeLimit        string
	MyNetworks              string
	RelayHost               string
	RelayUser               bool
	RecipientDelimiter      string
	OutboundTLSLevel        string
	DefersOnTLSError        bool
	RejectUnlistedRecipient string
	GatewayAddress          string
	MailFilterAddress       string
	BackendAddress          string
	AuthorizedXclient       string
	PostfixLogFile          string
}

// loadPostfixConfig reads the environment like the legacy launcher +
// the legacy launcher's env cleanup did.
func loadPostfixConfig() (PostfixConfig, error) {
	cfg := PostfixConfig{
		Domain:                  agent.Getenv("DOMAIN", "example.com"),
		MessageSizeLimit:        agent.Getenv("MESSAGE_SIZE_LIMIT", "50000000"),
		RelayHost:               agent.Getenv("RELAYHOST", ""),
		RelayUser:               os.Getenv("RELAYUSER") != "",
		RecipientDelimiter:      agent.Getenv("RECIPIENT_DELIMITER", ""),
		OutboundTLSLevel:        agent.Getenv("OUTBOUND_TLS_LEVEL", "dane"),
		DefersOnTLSError:        envTrue("DEFER_ON_TLS_ERROR", true),
		RejectUnlistedRecipient: agent.Getenv("REJECT_UNLISTED_RECIPIENT", "no"),
		GatewayAddress:          agent.Getenv("GATEWAY_ADDRESS", "gateway"),
		MailFilterAddress:       agent.Getenv("MAIL_FILTER_ADDRESS", "mail-filter"),
		BackendAddress:          agent.Getenv("BACKEND_ADDRESS", "backend"),
		PostfixLogFile:          os.Getenv("POSTFIX_LOG_FILE"),
	}

	hostnames := agent.Getenv("HOSTNAMES", "")
	if hostnames == "" {
		return cfg, fmt.Errorf("HOSTNAMES is required")
	}
	cfg.Hostname = strings.TrimSpace(strings.Split(hostnames, ",")[0])

	subnet, err := agent.CIDR("SUBNET")
	if err != nil {
		return cfg, err
	}
	subnet6 := os.Getenv("SUBNET6")

	// Postfix requires IPv6 addresses to be wrapped in square brackets
	// (the legacy launcher did the same re.sub on RELAYNETS and the template brackets
	// SUBNET6).
	parts := []string{"127.0.0.1/32", subnet}
	if subnet6 != "" {
		parts = append(parts, "[::1]/128", bracketCIDR(subnet6))
	}
	if relnets := os.Getenv("RELAYNETS"); relnets != "" {
		for _, n := range strings.Split(relnets, ",") {
			if n = strings.TrimSpace(n); n != "" {
				parts = append(parts, bracketIPv6Prefix(n))
			}
		}
	}
	cfg.MyNetworks = strings.Join(parts, " ")

	xclient := subnet
	if subnet6 != "" {
		xclient += "," + bracketCIDR(subnet6)
	}
	cfg.AuthorizedXclient = xclient

	return cfg, nil
}

// bracketCIDR formats "fdc4:.../64" as "[fdc4:...]/64" (brackets stripped
// from the address part, matching the template's SUBNET6.translate()).
func bracketCIDR(cidr string) string {
	host, rest, ok := strings.Cut(strings.TrimPrefix(strings.TrimSuffix(cidr, "]"), "["), "/")
	if !ok {
		return "[" + host + "]"
	}
	return "[" + host + "]/" + rest
}

// bracketIPv6Prefix wraps bare IPv6 prefixes from RELAYNETS in brackets,
// replicating the legacy launcher's re.sub(r'([0-9a-fA-F]+:[0-9a-fA-F:]+)/', '[\\1]/').
func bracketIPv6Prefix(net string) string {
	if strings.Contains(net, ":") && !strings.HasPrefix(net, "[") {
		if host, rest, ok := strings.Cut(net, "/"); ok {
			return "[" + host + "]/" + rest
		}
	}
	return net
}

func envTrue(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	}
	return def
}


