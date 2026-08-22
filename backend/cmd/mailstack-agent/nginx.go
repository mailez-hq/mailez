package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mailez/backend/internal/agent"
)

func runNginx() error {
	cfg, err := loadNginxConfig()
	if err != nil {
		return err
	}

	// Rebuild nginx/DANE chains from the ACME fullchains (letsencrypt only).
	if cfg.TLSFlavor == "letsencrypt" || cfg.TLSFlavor == "mail-letsencrypt" {
		type chainFile struct{ src, dst string; strip bool }
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

	// Stale pid cleanup (start.py did the same).
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

// acmeLoop keeps certificates fresh. TODO(M2b): replace the certbot CLI with
// an embedded lego client so the image no longer needs the Python certbot.
func acmeLoop(cfg NginxConfig) {
	time.Sleep(5 * time.Second) // let nginx start
	for {
		runCertbot(cfg, false)
		runCertbot(cfg, true)
		if err := regenerateChains(); err != nil {
			fmt.Fprintf(os.Stderr, "nginx: regenerate chains after renew: %v\n", err)
		}
		_ = exec.Command("/usr/sbin/nginx", "-s", "reload").Run()
		time.Sleep(24 * time.Hour)
	}
}

func runCertbot(cfg NginxConfig, ecdsa bool) {
	hostnames := strings.Join(strings.Fields(cfgHostnames()), ",")
	certName := "mailez"
	keyType := "rsa"
	if ecdsa {
		certName = "mailez-ecdsa"
		keyType = "ecdsa"
	}
	cmd := exec.Command("certbot",
		"-n", "--agree-tos",
		"-d", hostnames, "--expand", "--allow-subset-of-names",
		"-m", fmt.Sprintf("%s@%s", os.Getenv("POSTMASTER"), os.Getenv("DOMAIN")),
		"certonly", "--standalone",
		"--cert-name", certName,
		"--preferred-challenges", "http", "--http-01-port", "8008",
		"--keep-until-expiring", "--allow-subset-of-names",
		"--key-type", keyType,
		"--renew-with-new-domains",
		"--config-dir", "/certs/letsencrypt",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nginx: certbot (%s) failed: %v\n", certName, err)
	}
}

func cfgHostnames() string {
	return agent.Getenv("HOSTNAMES", "")
}

func regenerateChains() error {
	for _, f := range []struct{ src, dst string; strip bool }{
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
