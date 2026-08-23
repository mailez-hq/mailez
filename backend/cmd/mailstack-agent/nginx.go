package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"mailez/backend/internal/agent"
)

func runNginx() error {
	cfg, err := loadNginxConfig()
	if err != nil {
		return err
	}

	// Rebuild nginx/DANE chains from the ACME fullchains (letsencrypt only).
	if cfg.TLSFlavor == "letsencrypt" || cfg.TLSFlavor == "mail-letsencrypt" {
		type chainFile struct {
			src, dst string
			strip    bool
		}
		for _, f := range []chainFile{
			{"/certs/letsencrypt/live/mailez/fullchain.pem", "/certs/letsencrypt/live/mailez/nginx-chain.pem", true},
			{"/certs/letsencrypt/live/mailez/fullchain.pem", "/certs/letsencrypt/live/mailez/DANE-chain.pem", false},
			{"/certs/letsencrypt/live/mailez-ecdsa/fullchain.pem", "/certs/letsencrypt/live/mailez-ecdsa/nginx-chain.pem", true},
			{"/certs/letsencrypt/live/mailez-ecdsa/fullchain.pem", "/certs/letsencrypt/live/mailez-ecdsa/DANE-chain.pem", false},
		} {
			if err := formatForNginx(f.src, f.dst, f.strip); err != nil {
				return err
			}
		}
	}

	if err := renderNginxConfigs(cfg); err != nil {
		return err
	}

	// Stale pid cleanup (the legacy launcher did the same).
	_ = os.Remove("/var/run/nginx.pid")

	// The dovecot login proxy daemonizes itself; nginx runs in the foreground.
	if err := exec.Command("/usr/sbin/dovecot", "-c", "/etc/dovecot/proxy.conf").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nginx: dovecot proxy failed to start: %v\n", err)
	}

	if cfg.TLSFlavor == "letsencrypt" || cfg.TLSFlavor == "mail-letsencrypt" {
		go acmeLoop(cfg)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return agent.RunChild(ctx, []string{"/usr/sbin/nginx", "-g", "daemon off;"})
}

func cfgHostnames() string {
	return agent.Getenv("HOSTNAMES", "")
}

func regenerateChains() error {
	for _, f := range []struct {
		src, dst string
		strip    bool
	}{
		{"/certs/letsencrypt/live/mailez/fullchain.pem", "/certs/letsencrypt/live/mailez/nginx-chain.pem", true},
		{"/certs/letsencrypt/live/mailez/fullchain.pem", "/certs/letsencrypt/live/mailez/DANE-chain.pem", false},
		{"/certs/letsencrypt/live/mailez-ecdsa/fullchain.pem", "/certs/letsencrypt/live/mailez-ecdsa/nginx-chain.pem", true},
		{"/certs/letsencrypt/live/mailez-ecdsa/fullchain.pem", "/certs/letsencrypt/live/mailez-ecdsa/DANE-chain.pem", false},
	} {
		if err := formatForNginx(f.src, f.dst, f.strip); err != nil {
			return err
		}
	}
	return nil
}
