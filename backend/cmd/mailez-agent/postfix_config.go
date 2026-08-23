package main

import (
	"fmt"
	"os"
	"strings"

	"mailez/backend/internal/agent"
)

// PostfixConfig is the typed, validated view of the environment consumed by
// the postfix templates (main.cf / master.cf / mta-sts-daemon.yml).
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

// loadPostfixConfig reads and normalizes the environment.
func loadPostfixConfig() (PostfixConfig, error) {
	cfg := PostfixConfig{
		Domain:                  agent.Getenv("MAILEZ_DOMAIN", "example.com"),
		MessageSizeLimit:        agent.Getenv("MAILEZ_MESSAGE_SIZE_LIMIT", "50000000"),
		RelayHost:               agent.Getenv("MAILEZ_RELAYHOST", ""),
		RelayUser:               os.Getenv("MAILEZ_RELAYUSER") != "",
		RecipientDelimiter:      agent.Getenv("MAILEZ_RECIPIENT_DELIMITER", ""),
		OutboundTLSLevel:        agent.Getenv("MAILEZ_OUTBOUND_TLS_LEVEL", "dane"),
		DefersOnTLSError:        envTrue("MAILEZ_DEFER_ON_TLS_ERROR", true),
		RejectUnlistedRecipient: agent.Getenv("MAILEZ_REJECT_UNLISTED_RECIPIENT", "no"),
		GatewayAddress:          agent.Getenv("MAILEZ_GATEWAY_ADDRESS", "gateway"),
		MailFilterAddress:       agent.Getenv("MAIL_FILTER_ADDRESS", "mail-filter"),
		BackendAddress:          agent.Getenv("MAILEZ_BACKEND_ADDRESS", "backend"),
		PostfixLogFile:          os.Getenv("POSTFIX_LOG_FILE"),
	}

	hostnames := agent.Getenv("MAILEZ_HOSTNAMES", "")
	if hostnames == "" {
		return cfg, fmt.Errorf("MAILEZ_HOSTNAMES is required")
	}
	cfg.Hostname = strings.TrimSpace(strings.Split(hostnames, ",")[0])

	subnet, err := agent.CIDR("MAILEZ_SUBNET")
	if err != nil {
		return cfg, err
	}
	subnet6 := os.Getenv("MAILEZ_SUBNET6")

	// Postfix requires IPv6 addresses to be wrapped in square brackets
	// (RELAYNETS entries and SUBNET6 are bracketed the same way).
	parts := []string{"127.0.0.1/32", subnet}
	if subnet6 != "" {
		parts = append(parts, "[::1]/128", bracketCIDR(subnet6))
	}
	if relnets := os.Getenv("MAILEZ_RELAYNETS"); relnets != "" {
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
// from the address part, matching the bracket form the templates expect).
func bracketCIDR(cidr string) string {
	host, rest, ok := strings.Cut(strings.TrimPrefix(strings.TrimSuffix(cidr, "]"), "["), "/")
	if !ok {
		return "[" + host + "]"
	}
	return "[" + host + "]/" + rest
}

// bracketIPv6Prefix wraps bare IPv6 prefixes from RELAYNETS in brackets
// (matches the [host]/prefix form postfix expects).
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
