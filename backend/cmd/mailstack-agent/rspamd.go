package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"mailez/backend/internal/agent"
)

func runRspamd() error {
	cfg, err := loadRspamdConfig()
	if err != nil {
		return err
	}
	files, err := renderRspamdAll(cfg)
	if err != nil {
		return err
	}

	// /overrides/* that do not collide with rendered configs are copied into
	// local.d.
	configNames := map[string]bool{}
	for dest := range files {
		configNames[filepath.Base(dest)] = true
	}
	if overrides, err := filepath.Glob("/overrides/*"); err == nil {
		for _, overrideFile := range overrides {
			base := filepath.Base(overrideFile)
			if configNames[base] {
				continue
			}
			data, err := os.ReadFile(overrideFile)
			if err != nil {
				return err
			}
			if err := agent.AtomicWrite(filepath.Join("/etc/rspamd/local.d", base), data, 0o644); err != nil {
				return err
			}
		}
	}
	if err := writeRenderedRspamd(files); err != nil {
		return err
	}

	// Wait for the control plane: the admin may not be up just yet. The
	// original retried every second forever; we use capped exponential backoff.
	backend := cfg.BackendAddress
	healthURL := "http://" + backend + ":8080/stack/rspamd/local_domains"
	client := &http.Client{Timeout: 2 * time.Second}
	delay := time.Second
	for {
		if resp, err := client.Get(healthURL); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		fmt.Fprintf(os.Stderr, "rspamd: control plane not ready, retrying in %s\n", delay)
		select {
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}

	if cfg.DmarcSendReports {
		if err := installDmarcCron(); err != nil {
			return err
		}
	}

	if err := os.MkdirAll("/run/rspamd", 0o755); err != nil {
		return err
	}
	rspamdUser, err := user.Lookup("rspamd")
	if err != nil {
		return fmt.Errorf("lookup rspamd user: %w", err)
	}
	uid, gid := lookupIDs(rspamdUser)
	if err := os.Chown("/run/rspamd", uid, gid); err != nil {
		return err
	}
	if err := chownRspamdData(uid, gid); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return agent.RunChild(ctx, []string{"/usr/bin/rspamd", "-f", "-u", "rspamd", "-g", "rspamd"})
}

// installDmarcCron writes the daily DMARC report job and starts crond
// (enabled when DMARC_SEND_REPORTS is set).
func installDmarcCron() error {
	script := "#!/bin/sh\n# Send DMARC reports for yesterday\nsu rspamd -s /bin/sh -c \"/usr/bin/rspamadm dmarc_report $(date -d @$(($(date +%s)-86400)) +%Y%m%d)\" >>/proc/1/fd/1 2>&1\n"
	if err := agent.AtomicWrite("/etc/periodic/daily/dmarc-reports", []byte(script), 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("/usr/sbin/crond").CombinedOutput(); err != nil {
		return fmt.Errorf("crond: %v\n%s", err, out)
	}
	return nil
}

// chownRspamdData replicates "find /var/lib/rspamd | grep -v /filter |
// xargs -n1 chown rspamd:rspamd" without touching the filter volume (it is
// owned by postfix so rspamd can drop quarantine files).
func chownRspamdData(uid, gid int) error {
	return filepath.Walk("/var/lib/rspamd", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if strings.Contains(p, "/filter") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return os.Chown(p, uid, gid)
	})
}

func lookupIDs(u *user.User) (int, int) {
	var uid, gid int
	fmt.Sscanf(u.Uid, "%d", &uid)
	fmt.Sscanf(u.Gid, "%d", &gid)
	return uid, gid
}
