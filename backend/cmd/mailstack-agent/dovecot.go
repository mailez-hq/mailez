package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"mailez/backend/internal/agent"
)

func runDovecot() error {
	cfg, err := loadDovecotConfig()
	if err != nil {
		return err
	}

	files, err := renderDovecotAll(cfg)
	if err != nil {
		return err
	}
	for dest, data := range files {
		perm := os.FileMode(0o644)
		if filepath.Dir(dest) == "/conf/bin" {
			perm = 0o555
		}
		if err := agent.AtomicWrite(dest, data, perm); err != nil {
			return err
		}
	}

	// Ownership fixes (start.py did the same).
	for _, cmd := range [][]string{
		{"chown", "mail:mail", "/mail"},
		{"chown", "-R", "mail:mail", "/var/lib/dovecot", "/conf"},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "dovecot: %s failed: %v\n%s\n", cmd[0], err, out)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// mailez: dovecot dict proxy -> control plane internal API.
	backend := agent.Getenv("BACKEND_ADDRESS", "backend")
	base := "http://" + backend + ":8080/internal/dovecot/"
	handler := agent.NewDictHandler(map[string]string{
		"quota": base + "{}",
		"auth":  base + "{}",
		"sieve": base + "{}",
	})
	go func() {
		if err := handler.Serve(ctx, "/tmp/mailez.socket"); err != nil {
			fmt.Fprintf(os.Stderr, "dovecot: mailez dict proxy: %v\n", err)
		}
	}()

	return agent.RunChild(ctx, []string{"/usr/sbin/dovecot", "-c", "/etc/dovecot/dovecot.conf", "-F"})
}
