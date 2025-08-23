package main

import (
	"os"

	"mailez/backend/internal/agent"
)

// RspamdConfig is the typed view of the environment consumed by the rspamd
// templates.
type RspamdConfig struct {
	Subnet              string
	Subnet6             string
	RelayNets           string
	ScanMacros          bool
	Antivirus           string
	AntivirusAction     string
	AntivirusAddress    string
	MacroScannerAddress string
	RedisAddress        string
	BackendAddress      string
	Postmaster          string
	Domain              string
	Sitename            string
	DmarcSendReports    bool
	MtaAddress          string
}

func loadRspamdConfig() (RspamdConfig, error) {
	cfg := RspamdConfig{
		Subnet:              agent.Getenv("MAILEZ_SUBNET", ""),
		Subnet6:             os.Getenv("MAILEZ_SUBNET6"),
		RelayNets:           os.Getenv("MAILEZ_RELAYNETS"),
		ScanMacros:          envTrue("MAILEZ_SCAN_MACROS", false),
		Antivirus:           agent.Getenv("MAILEZ_ANTIVIRUS", "none"),
		AntivirusAction:     agent.Getenv("MAILEZ_ANTIVIRUS_ACTION", "discard"),
		AntivirusAddress:    agent.Getenv("MAILEZ_ANTIVIRUS_ADDRESS", "antivirus"),
		MacroScannerAddress: agent.Getenv("MACRO_SCANNER_ADDRESS", "macro-scanner"),
		RedisAddress:        agent.Getenv("MAILEZ_REDIS_ADDRESS", "redis"),
		BackendAddress:      agent.Getenv("MAILEZ_BACKEND_ADDRESS", "backend"),
		Postmaster:          agent.Getenv("MAILEZ_POSTMASTER", "postmaster"),
		Domain:              agent.Getenv("MAILEZ_DOMAIN", "example.com"),
		Sitename:            agent.Getenv("MAILEZ_SITENAME", ""),
		DmarcSendReports:    envTrue("MAILEZ_DMARC_SEND_REPORTS", false),
		MtaAddress:          agent.Getenv("MTA_ADDRESS", "mta"),
	}
	if cfg.Subnet == "" {
		var err error
		cfg.Subnet, err = agent.CIDR("MAILEZ_SUBNET")
		if err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}
