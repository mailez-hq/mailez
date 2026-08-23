package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"mailez/backend/internal/agent"
)

// TLSPaths holds the certificate paths rendered into the TLS configs.
type TLSPaths struct {
	Cert    string
	Key     string
	AltCert string
	AltKey  string
}

// NginxConfig is the typed, validated view of the environment consumed by the
// nginx/dovecot-proxy templates.
type NginxConfig struct {
	Resolver             string
	Hostname             string
	RealIPHeader         string
	RealIPFrom           string
	RealIPFromList       []string
	ProxyProtocol25      bool
	ProxyProtocol80      bool
	ProxyProtocol110     bool
	ProxyProtocol143     bool
	ProxyProtocol443     bool
	ProxyProtocol465     bool
	ProxyProtocol587     bool
	ProxyProtocol993     bool
	ProxyProtocol995     bool
	ProxyProtocol4190    bool
	Subnet6              bool
	TLSFlavor            string
	TLSError             bool
	TLSPermissive        bool
	TLS                  *TLSPaths
	Port80               bool
	Port143              bool
	Port110              bool
	Port587              bool
	Port4190             bool
	Port995              bool
	TLS443               bool
	TLS993               bool
	TLS995               bool
	TLS465               bool
	BackendAddress       string
	MailFilterAddress    string
	MessageSizeLimit     int
	MessageSizeLimitPlus int
	CPUCount             int
	API                  bool
	Postmaster           string
	Domain               string
	PostfixAddress       string
	RecipientDelimiter   string
}

var (
	protoMail         = []string{"25", "110", "995", "143", "993", "587", "465", "4190"}
	portsRequiringTLS = []string{"443", "465", "993", "995"}
	defaultPorts      = "25,80,443,465,993,995,4190"
	defaultTLS        = "25,80,443,465,993,995,4190"
)

func loadNginxConfig() (NginxConfig, error) {
	cfg := NginxConfig{
		BackendAddress:     agent.Getenv("MAILEZ_BACKEND_ADDRESS", "backend"),
		MailFilterAddress:  agent.Getenv("MAIL_FILTER_ADDRESS", "mail-filter"),
		PostfixAddress:     agent.Getenv("POSTFIX_ADDRESS", "postfix"),
		RecipientDelimiter: agent.Getenv("MAILEZ_RECIPIENT_DELIMITER", "+"),
		RealIPHeader:       os.Getenv("REAL_IP_HEADER"),
		RealIPFrom:         os.Getenv("REAL_IP_FROM"),
		TLSFlavor:          agent.Getenv("MAILEZ_TLS", "off"),
		TLSPermissive:      envBool("TLS_PERMISSIVE", false),
		API:                envBool("MAILEZ_API", true),
		Postmaster:         agent.Getenv("MAILEZ_POSTMASTER", "postmaster"),
		Domain:             agent.Getenv("MAILEZ_DOMAIN", "example.com"),
		Subnet6:            os.Getenv("MAILEZ_SUBNET6") != "",
	}

	hostnames := agent.Getenv("MAILEZ_HOSTNAMES", "")
	if hostnames == "" {
		return cfg, fmt.Errorf("MAILEZ_HOSTNAMES is required")
	}
	cfg.Hostname = strings.TrimSpace(strings.Split(hostnames, ",")[0])

	if cfg.RealIPFrom != "" {
		for _, ip := range strings.Split(cfg.RealIPFrom, ",") {
			if ip = strings.TrimSpace(ip); ip != "" {
				cfg.RealIPFromList = append(cfg.RealIPFromList, ip)
			}
		}
	}

	var err error
	if cfg.MessageSizeLimit, err = strconv.Atoi(agent.Getenv("MAILEZ_MESSAGE_SIZE_LIMIT", "50000000")); err != nil {
		return cfg, fmt.Errorf("MAILEZ_MESSAGE_SIZE_LIMIT: %w", err)
	}
	cfg.MessageSizeLimitPlus = cfg.MessageSizeLimit + 8388608

	if cfg.Resolver, err = resolverAddress(); err != nil {
		return cfg, err
	}

	cfg.CPUCount = runtime.NumCPU()
	if v := os.Getenv("MAILEZ_CPU_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CPUCount = n
		}
	}

	applyProxyProtocol(&cfg)
	applyPorts(&cfg)
	applyTLS(&cfg)
	return cfg, nil
}

// applyProxyProtocol reproduces the PROXY_PROTOCOL handling from the
// environment (per-port proxy-protocol flags).
func applyProxyProtocol(cfg *NginxConfig) {
	set := func(port string, v bool) {
		switch port {
		case "25":
			cfg.ProxyProtocol25 = v
		case "80":
			cfg.ProxyProtocol80 = v
		case "110":
			cfg.ProxyProtocol110 = v
		case "143":
			cfg.ProxyProtocol143 = v
		case "443":
			cfg.ProxyProtocol443 = v
		case "465":
			cfg.ProxyProtocol465 = v
		case "587":
			cfg.ProxyProtocol587 = v
		case "993":
			cfg.ProxyProtocol993 = v
		case "995":
			cfg.ProxyProtocol995 = v
		case "4190":
			cfg.ProxyProtocol4190 = v
		}
	}
	all := func(ports []string) {
		for _, p := range ports {
			set(p, true)
		}
	}
	for _, item := range strings.Split(os.Getenv("MAILEZ_PROXY_PROTOCOL"), ",") {
		switch strings.TrimSpace(item) {
		case "mail":
			all(protoMail)
		case "all-but-http":
			all(append(append([]string{}, protoMail...), "443"))
		case "all":
			all(append(append(append([]string{}, protoMail...), "443"), "80"))
		case "":
		default:
			if isPort(item) {
				set(strings.TrimSpace(item), true)
			}
		}
	}
}

// applyPorts reproduces the port selection from the environment.
func applyPorts(cfg *NginxConfig) {
	plain := cfg.TLSFlavor == "off"
	setPort := func(port string) {
		switch port {
		case "80":
			cfg.Port80 = true
		case "110":
			cfg.Port110 = true
		case "143":
			cfg.Port143 = true
		case "587":
			cfg.Port587 = true
		case "4190":
			cfg.Port4190 = true
		case "995":
			cfg.Port995 = true
		}
	}
	for _, item := range strings.Split(agent.Getenv("MAILEZ_PORTS", defaultPorts), ",") {
		item = strings.TrimSpace(item)
		if !isPort(item) {
			continue
		}
		if plain && contains(portsRequiringTLS, item) {
			continue
		}
		setPort(item)
	}
	if cfg.TLSFlavor != "off" {
		for _, item := range strings.Split(agent.Getenv("MAILEZ_TLS_PORTS", defaultTLS), ",") {
			item = strings.TrimSpace(item)
			if !isPort(item) || !contains(portsRequiringTLS, item) {
				continue
			}
			switch item {
			case "443":
				cfg.TLS443 = true
			case "993":
				cfg.TLS993 = true
			case "995":
				cfg.TLS995 = true
			case "465":
				cfg.TLS465 = true
			}
		}
	}
}

// applyTLS builds the TLS paths and the TLS_ERROR state.
func applyTLS(cfg *NginxConfig) {
	switch cfg.TLSFlavor {
	case "cert":
		cert := agent.Getenv("MAILEZ_TLS_CERT_FILE", "cert.pem")
		key := agent.Getenv("MAILEZ_TLS_KEY_FILE", "key.pem")
		cfg.TLS = &TLSPaths{Cert: "/certs/" + cert, Key: "/certs/" + key}
	case "letsencrypt":
		cfg.TLS = &TLSPaths{
			Cert:    "/certs/letsencrypt/live/mailez/nginx-chain.pem",
			Key:     "/certs/letsencrypt/live/mailez/privkey.pem",
			AltCert: "/certs/letsencrypt/live/mailez-ecdsa/nginx-chain.pem",
			AltKey:  "/certs/letsencrypt/live/mailez-ecdsa/privkey.pem",
		}
	case "off":
		cfg.TLS = nil
	default:
		cfg.TLSError = true
		cfg.TLS = nil
	}
	if cfg.TLS != nil {
		for _, p := range []string{cfg.TLS.Cert, cfg.TLS.Key} {
			if _, err := os.Stat(p); err != nil {
				cfg.TLSError = true
				break
			}
		}
	}
}

func firstNameserver(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			ns := fields[1]
			if strings.Contains(ns, ":") && !strings.HasPrefix(ns, "[") {
				ns = "[" + ns + "]"
			}
			return ns, nil
		}
	}
	return "", fmt.Errorf("no nameserver found in %s", path)
}

// resolverAddress prefers the explicit MAILEZ_RESOLVER_ADDRESS env (e.g. the unbound
// container) and otherwise falls back to the container's first nameserver.
func resolverAddress() (string, error) {
	if v := os.Getenv("MAILEZ_RESOLVER_ADDRESS"); v != "" {
		if strings.Contains(v, ":") && !strings.HasPrefix(v, "[") {
			v = "[" + v + "]"
		}
		return v, nil
	}
	return firstNameserver("/etc/resolv.conf")
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	}
	return def
}

func isPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n <= 65535
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
