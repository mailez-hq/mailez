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
		{"chown", "-R", "mail:mail", "/var/lib/legacy IMAP", "/conf"},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "legacy IMAP: %s failed: %v\n%s\n", cmd[0], err, out)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// mailez: legacy IMAP dict proxy -> control plane internal API.
	backend := agent.Getenv("BACKEND_ADDRESS", "backend")
	base := "http://" + backend + ":8080/internal/legacy IMAP/"
	handler := agent.NewDictHandler(map[string]string{
		"quota": base + "{}",
		"auth":  base + "{}",
		"sieve": base + "{}",
	})
	go func() {
		if err := handler.Serve(ctx, "/tmp/mailez.socket"); err != nil {
			fmt.Fprintf(os.Stderr, "legacy IMAP: mailez dict proxy: %v\n", err)
		}
	}()

	return agent.RunChild(ctx, []string{"/usr/sbin/legacy IMAP", "-c", "/etc/legacy IMAP/dovecot.conf", "-F"})
}
