package main

import (
	"os"

	"mailez/backend/internal/agent"
)

// RspamdConfig is the typed view of the environment consumed by the rspamd
// templates (the vendored start.py rendered every /conf file through Jinja).
type RspamdConfig struct {
	Subnet           string
	Subnet6          string
	RelayNets        string
	ScanMacros       bool
	Antivirus        string
	AntivirusAction  string
	AntivirusAddress string
	OletoolsAddress  string
	RedisAddress     string
	AdminAddress     string
	Postmaster       string
	Domain           string
	Sitename         string
	DmarcSendReports bool
	SmtpAddress      string
}

func loadRspamdConfig() (RspamdConfig, error) {
	cfg := RspamdConfig{
		Subnet:           agent.Getenv("SUBNET", ""),
		Subnet6:          os.Getenv("SUBNET6"),
		RelayNets:        os.Getenv("RELAYNETS"),
		ScanMacros:       envTrue("SCAN_MACROS", false),
		Antivirus:        agent.Getenv("ANTIVIRUS", "none"),
		AntivirusAction:  agent.Getenv("ANTIVIRUS_ACTION", "discard"),
		AntivirusAddress: agent.Getenv("ANTIVIRUS_ADDRESS", "antivirus"),
		OletoolsAddress:  agent.Getenv("OLETOOLS_ADDRESS", "oletools"),
		RedisAddress:     agent.Getenv("REDIS_ADDRESS", "redis"),
		AdminAddress:     agent.Getenv("ADMIN_ADDRESS", "admin"),
		Postmaster:       agent.Getenv("POSTMASTER", "postmaster"),
		Domain:           agent.Getenv("DOMAIN", "example.com"),
		Sitename:         agent.Getenv("SITENAME", ""),
		DmarcSendReports: envTrue("DMARC_SEND_REPORTS", false),
		SmtpAddress:      agent.Getenv("SMTP_ADDRESS", "smtp"),
	}
	if cfg.Subnet == "" {
		var err error
		cfg.Subnet, err = agent.CIDR("SUBNET")
		if err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}
