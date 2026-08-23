package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"mailez/backend/internal/agent"
)

// DovecotConfig is the typed view of the environment consumed by the dovecot
// templates.
type DovecotConfig struct {
	Postmaster            string
	Domain                string
	Hostname              string
	ProxyProtocol25       bool
	GatewayAddress        string
	Subnet6               bool
	Subnet                string
	CPUCount              int
	RecipientDelimiter    string
	MailPlugins           string
	DefaultMailboxes      []string
	FTSEnabled            bool
	FTSLanguages          string
	FTSTika               bool
	FTSAttachmentsAddress string
	CompressionEnabled    bool
	Compression           string
	CompressionLevel      string
}

func loadDovecotConfig() (DovecotConfig, error) {
	cfg := DovecotConfig{
		Postmaster:            agent.Getenv("POSTMASTER", "postmaster"),
		Domain:                agent.Getenv("DOMAIN", "example.com"),
		GatewayAddress:        agent.Getenv("GATEWAY_ADDRESS", "gateway"),
		Subnet:                agent.Getenv("SUBNET", "192.168.206.0/24"),
		RecipientDelimiter:    agent.Getenv("RECIPIENT_DELIMITER", "+"),
		DefaultMailboxes:      []string{"Trash", "Drafts", "Sent", "Junk"},
		Compression:           os.Getenv("COMPRESSION"),
		CompressionLevel:      os.Getenv("COMPRESSION_LEVEL"),
		FTSAttachmentsAddress: agent.Getenv("FTS_ATTACHMENTS_ADDRESS", "tika"),
	}

	hostnames := agent.Getenv("HOSTNAMES", "")
	if hostnames == "" {
		return cfg, errRequired("HOSTNAMES")
	}
	cfg.Hostname = strings.TrimSpace(strings.Split(hostnames, ",")[0])

	if v := os.Getenv("SUBNET6"); v != "" {
		cfg.Subnet6 = true
		cfg.Subnet += " " + v
	}
	if v := os.Getenv("PROXY_PROTOCOL"); strings.Contains(v, "25") || v == "mail" || v == "all-but-http" || v == "all" {
		cfg.ProxyProtocol25 = true
	}

	cfg.CPUCount = runtime.NumCPU()
	if v := os.Getenv("CPU_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CPUCount = n
		}
	}

	fts := strings.ToLower(os.Getenv("FULL_TEXT_SEARCH"))
	cfg.FTSEnabled = fts != "" && fts != "off" && fts != "false" && fts != "0"
	if cfg.FTSEnabled {
		if fts == "" {
			cfg.FTSLanguages = "en"
		} else {
			fields := strings.Split(os.Getenv("FULL_TEXT_SEARCH"), ",")
			for i := range fields {
				fields[i] = strings.TrimSpace(fields[i])
			}
			cfg.FTSLanguages = strings.Join(fields, " ")
		}
	}
	cfg.FTSTika = cfg.FTSEnabled && envTruthy(os.Getenv("FULL_TEXT_SEARCH_ATTACHMENTS"))

	switch cfg.Compression {
	case "gz", "bz2", "lz4", "zstd":
		cfg.CompressionEnabled = true
	}

	plugins := "quota quota_clone"
	if cfg.CompressionEnabled {
		plugins += " zlib"
	}
	if cfg.FTSEnabled {
		plugins += " fts fts_flatcurve"
	}
	cfg.MailPlugins = plugins
	return cfg, nil
}

type requiredError struct{ key string }

func (e requiredError) Error() string {
	return "required environment variable " + e.key + " is not set"
}

func errRequired(key string) error { return requiredError{key: key} }

func envTruthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "true" || v == "yes" || v == "1"
}
