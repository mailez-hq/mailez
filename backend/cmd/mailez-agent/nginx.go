package main

import (
	"context"
	"os"
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

	// The mailezine engine authenticates by itself; the gateway runs
	// nginx (HTTP + ACME) in the foreground only.
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
