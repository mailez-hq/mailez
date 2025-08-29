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

// NginxConfig is the typed, validated view of the environment consumed by
// the nginx templates (HTTP/ACME gateway only; the engine owns the mail
// ports).
type NginxConfig struct {
	Resolver             string
	Hostname             string
	RealIPHeader         string
	RealIPFrom           string
	RealIPFromList       []string
	ProxyProtocol80      bool
	ProxyProtocol443     bool
	Subnet6              bool
	TLSFlavor            string
	TLSError             bool
	TLSPermissive        bool
	TLS                  *TLSPaths
	Port80               bool
	TLS443               bool
	BackendAddress       string
	MailFilterAddress    string
	MessageSizeLimit     int
	MessageSizeLimitPlus int
	CPUCount             int
	API                  bool
	Postmaster           string
	Domain               string
	Engine               string // mailezine (the only engine; kept for template compat)
	RecipientDelimiter   string
}

var (
	portsRequiringTLS = []string{"443"}
	defaultPorts      = "80,443"
	defaultTLS        = "80,443"
)

func loadNginxConfig() (NginxConfig, error) {
	cfg := NginxConfig{
		BackendAddress:     agent.Getenv("MAILEZ_BACKEND_ADDRESS", "backend"),
		MailFilterAddress:  agent.Getenv("MAIL_FILTER_ADDRESS", "mail-filter"),
		Engine:             agent.Getenv("MAILEZ_ENGINE", "mailezine"),
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
	if cfg.Engine != "mailezine" {
		return cfg, fmt.Errorf("MAILEZ_ENGINE must be mailezine, got %q", cfg.Engine)
	}
	return cfg, nil
}

// applyProxyProtocol reproduces the PROXY_PROTOCOL handling from the
// environment (per-port proxy-protocol flags; HTTP ports only — the mail
// ports belong to the mailezine engine now).
func applyProxyProtocol(cfg *NginxConfig) {
	set := func(port string, v bool) {
		switch port {
		case "80":
			cfg.ProxyProtocol80 = v
		case "443":
			cfg.ProxyProtocol443 = v
		}
	}
	for _, item := range strings.Split(os.Getenv("MAILEZ_PROXY_PROTOCOL"), ",") {
		switch strings.TrimSpace(item) {
		case "all-but-http":
			set("443", true)
		case "all":
			set("443", true)
			set("80", true)
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
	for _, item := range strings.Split(agent.Getenv("MAILEZ_PORTS", defaultPorts), ",") {
		item = strings.TrimSpace(item)
		if !isPort(item) {
			continue
		}
		if plain && contains(portsRequiringTLS, item) {
			continue
		}
		if item == "80" {
			cfg.Port80 = true
		}
	}
	if cfg.TLSFlavor != "off" {
		for _, item := range strings.Split(agent.Getenv("MAILEZ_TLS_PORTS", defaultTLS), ",") {
			item = strings.TrimSpace(item)
			if item == "443" {
				cfg.TLS443 = true
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
