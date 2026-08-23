package main

import (
	"strings"
	"testing"
)

func TestRspamdRender(t *testing.T) {
	t.Setenv("SUBNET", "192.168.206.0/24")
	t.Setenv("SUBNET6", "fdc4:f303:9324::254/64")
	t.Setenv("RELAYNETS", "10.0.0.0/8")
	t.Setenv("BACKEND_ADDRESS", "backend")
	t.Setenv("REDIS_ADDRESS", "redis")
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("POSTMASTER", "postmaster")
	t.Setenv("SCAN_MACROS", "true")
	t.Setenv("ANTIVIRUS", "none")
	t.Setenv("DMARC_SEND_REPORTS", "false")

	cfg, err := loadRspamdConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	files, err := renderRspamdAll(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(files) != 25 {
		t.Fatalf("expected 25 rendered files, got %d", len(files))
	}

	checks := map[string][]string{
		"/etc/rspamd/local.d/options.inc": {"local_networks = [192.168.206.0/24, fdc4:f303:9324::254/64, 10.0.0.0/8];"},
		"/etc/rspamd/local.d/worker-controller.inc": {
			"bind_socket = \"*:11334\";",
			"secure_ip = \"192.168.206.0/24\";",
			"secure_ip = \"fdc4:f303:9324::254/64\";",
		},
		"/etc/rspamd/local.d/arc.conf":               {`vault_url = "http://backend:8080/internal/rspamd/vault";`},
		"/etc/rspamd/local.d/dkim_signing.conf":      {`vault_url = "http://backend:8080/internal/rspamd/vault";`},
		"/etc/rspamd/local.d/multimap.conf":          {`map = "http://backend:8080/internal/rspamd/local_domains";`},
		"/etc/rspamd/local.d/redis.conf":             {`servers = "redis";`},
		"/etc/rspamd/local.d/external_services.conf": {`servers = "macro-scanner:11343";`},
		"/etc/rspamd/local.d/dmarc.conf":             {"enabled = false;"},
		"/etc/rspamd/local.d/antivirus.conf":         {`.include(try=true,priority=1,duplicate=merge) "/overrides/antivirus.conf"`},
	}
	for file, wants := range checks {
		data, ok := files[file]
		if !ok {
			t.Fatalf("missing rendered file %s", file)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q", file, want)
			}
		}
	}

	// SCAN_MACROS=false drops the oletools blocks but keeps the includes.
	t.Setenv("SCAN_MACROS", "false")
	cfg2, _ := loadRspamdConfig()
	files2, err := renderRspamdAll(cfg2)
	if err != nil {
		t.Fatalf("render 2: %v", err)
	}
	if strings.Contains(string(files2["/etc/rspamd/local.d/external_services.conf"]), "oletools {") {
		t.Errorf("oletools block should be absent when SCAN_MACROS=false")
	}
	if !strings.Contains(string(files2["/etc/rspamd/local.d/composites.conf"]), ".include(try=true; priority=1; duplicate=merge)") {
		t.Errorf("composites include missing")
	}
}

func TestRspamdRenderClamav(t *testing.T) {
	t.Setenv("SUBNET", "192.168.206.0/24")
	t.Setenv("ANTIVIRUS", "clamav")
	t.Setenv("ANTIVIRUS_ACTION", "reject")
	cfg, err := loadRspamdConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	files, err := renderRspamdAll(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	av := string(files["/etc/rspamd/local.d/antivirus.conf"])
	for _, want := range []string{`servers = "antivirus:3310";`, `action = "reject"`} {
		if !strings.Contains(av, want) {
			t.Errorf("antivirus.conf missing %q:\n%s", want, av)
		}
	}
}
