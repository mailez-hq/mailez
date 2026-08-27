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

	// Rebuild the nginx chains from the ACME fullchains (letsencrypt only).
	if cfg.TLSFlavor == "letsencrypt" {
		type chainFile struct {
			src, dst string
			strip    bool
		}
		for _, f := range []chainFile{
			{"/certs/letsencrypt/live/mailez/fullchain.pem", "/certs/letsencrypt/live/mailez/nginx-chain.pem", true},
			{"/certs/letsencrypt/live/mailez-ecdsa/fullchain.pem", "/certs/letsencrypt/live/mailez-ecdsa/nginx-chain.pem", true},
		} {
			if err := formatForNginx(f.src, f.dst, f.strip); err != nil {
				return err
			}
		}
	}

	if err := renderNginxConfigs(cfg); err != nil {
		return err
	}

	// Stale pid cleanup.
	_ = os.Remove("/var/run/nginx.pid")

	if cfg.TLSFlavor == "letsencrypt" {
		go acmeLoop(cfg)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// postdove mode: the dovecot login proxy daemonizes itself; in mailezine
	// mode the engine authenticates by itself, so only nginx runs. Either
	// way nginx (HTTP + ACME) stays in the foreground.
	if cfg.Engine != "mailezine" {
		if err := exec.Command("/usr/sbin/dovecot", "-c", "/etc/dovecot/proxy.conf").Run(); err != nil {
			fmt.Fprintf(os.Stderr, "nginx: dovecot proxy failed to start: %v\n", err)
		}
	}
	return agent.RunChild(ctx, []string{"/usr/sbin/nginx", "-g", "daemon off;"})
}

func cfgHostnames() string {
	return agent.Getenv("MAILEZ_HOSTNAMES", "")
}

func regenerateChains() error {
	for _, f := range []struct {
		src, dst string
		strip    bool
	}{
		{"/certs/letsencrypt/live/mailez/fullchain.pem", "/certs/letsencrypt/live/mailez/nginx-chain.pem", true},
		{"/certs/letsencrypt/live/mailez-ecdsa/fullchain.pem", "/certs/letsencrypt/live/mailez-ecdsa/nginx-chain.pem", true},
	} {
		if err := formatForNginx(f.src, f.dst, f.strip); err != nil {
			return err
		}
	}
	return nil
}
