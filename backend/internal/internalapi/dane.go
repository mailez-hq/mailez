package internalapi

import (
	"os"

	"github.com/miekg/dns"
)

// hasDaneRecord reports whether a TLSA record exists for SMTP to a domain,
// queried through the system resolver (resolv.conf) or a fallback resolver.
func hasDaneRecord(domain string) bool {
	msg := new(dns.Msg)
	msg.SetQuestion("_25._tcp."+domain+".", dns.TypeTLSA)
	server := dnsServer()
	if server == "" {
		return false
	}
	client := new(dns.Client)
	resp, _, err := client.Exchange(msg, server)
	if err != nil {
		return false
	}
	return len(resp.Answer) > 0
}

func dnsServer() string {
	if cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(cfg.Servers) > 0 {
		return cfg.Servers[0] + ":" + cfg.Port
	}
	// Fall back to a public resolver when no resolv.conf is available.
	if os.Getenv("MAILESS_DNS") != "" {
		return os.Getenv("MAILESS_DNS")
	}
	return "8.8.8.8:53"
}
