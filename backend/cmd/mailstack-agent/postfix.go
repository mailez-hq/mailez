package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"mailez/backend/internal/agent"
)

// postfixTables maps socketmap table names to the control-plane URL suffix.
var postfixTables = map[string]string{
	"transport":    "transport/",
	"alias":        "alias/",
	"dane":         "dane/",
	"domain":       "domain/",
	"mailbox":      "mailbox/",
	"recipientmap": "recipient/map/",
	"sendermap":    "sender/map/",
	"senderlogin":  "sender/login/",
	"senderrate":   "sender/rate/",
}

func runPostfix() error {
	cfg, err := loadPostfixConfig()
	if err != nil {
		return err
	}

	files, err := renderPostfixAll(cfg)
	if err != nil {
		return err
	}
	for dest, data := range files {
		if err := agent.AtomicWrite(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}

	// Stale master.pid cleanup (see flockRemoveStaleMasterPID).
	flockRemoveStaleMasterPID()

	// Overrides are applied as: postconf lines, extra maps, and a full
	// mta-sts-daemon.yml replacement.
	if err := applyPostfixOverrides(cfg); err != nil {
		return err
	}

	// Postfix needs an existing (possibly empty) lmdb map for these lookups.
	for _, policy := range []string{"tls_policy", "transport"} {
		lmdb := filepath.Join("/etc/postfix", policy+".map.lmdb")
		if _, err := os.Stat(lmdb); err != nil {
			mapFile := filepath.Join("/etc/postfix", policy+".map")
			if err := os.WriteFile(mapFile, nil, 0o644); err != nil {
				return err
			}
			if out, err := exec.Command("postmap", mapFile).CombinedOutput(); err != nil {
				return fmt.Errorf("postmap %s: %v\n%s", mapFile, err, out)
			}
		}
	}

	if cfg.PostfixLogFile != "" {
		if err := renderPostfixLogrotate(cfg); err != nil {
			return err
		}
		if out, err := exec.Command("/usr/sbin/crond").CombinedOutput(); err != nil {
			return fmt.Errorf("crond: %v\n%s", err, out)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	backend := cfg.BackendAddress
	base := "http://" + backend + ":8080/internal/postfix/"
	urlFor := func(table, key string) string {
		suffix, ok := postfixTables[table]
		if !ok {
			return ""
		}
		return base + suffix + escapePath(key)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	go func() {
		if err := agent.PostfixSocketmapServe(ctx, "/tmp/mailez.socket", urlFor, client); err != nil {
			fmt.Fprintf(os.Stderr, "postfix: mailez socketmap: %v\n", err)
		}
	}()
	go func() {
		if err := serveMTASTS(ctx, "/etc/mta-sts-daemon.yml"); err != nil {
			fmt.Fprintf(os.Stderr, "postfix: mta-sts: %v\n", err)
		}
	}()

	for _, cmd := range [][]string{
		{"/usr/libexec/postfix/post-install", "meta_directory=/etc/postfix", "create-missing"},
		{"postfix", "set-permissions"},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %v\n%s", cmd[0], err, out)
		}
	}

	return agent.RunChild(ctx, []string{"postfix", "start-fg"})
}

// applyPostfixOverrides applies /overrides: postfix.cf and postfix.master
// lines are fed to postconf, *.map files are compiled with postmap, and an
// mta-sts-daemon.yml overrides the rendered one.
func applyPostfixOverrides(cfg PostfixConfig) error {
	if data, err := os.ReadFile("/overrides/postfix.cf"); err == nil {
		for _, line := range nonCommentLines(string(data)) {
			if out, err := exec.Command("postconf", "-e", line).CombinedOutput(); err != nil {
				return fmt.Errorf("postconf -e %q: %v\n%s", line, err, out)
			}
		}
	}
	if data, err := os.ReadFile("/overrides/postfix.master"); err == nil {
		for _, line := range nonCommentLines(string(data)) {
			if out, err := exec.Command("postconf", "-Me", line).CombinedOutput(); err != nil {
				return fmt.Errorf("postconf -Me %q: %v\n%s", line, err, out)
			}
		}
	}
	if maps, err := filepath.Glob("/overrides/*.map"); err == nil {
		for _, mapFile := range maps {
			dest := filepath.Join("/etc/postfix", filepath.Base(mapFile))
			if err := copyFile(mapFile, dest); err != nil {
				return err
			}
			if out, err := exec.Command("postmap", dest).CombinedOutput(); err != nil {
				return fmt.Errorf("postmap %s: %v\n%s", dest, err, out)
			}
			_ = os.Remove(dest)
		}
	}
	if data, err := os.ReadFile("/overrides/mta-sts-daemon.yml"); err == nil {
		if err := agent.AtomicWrite("/etc/mta-sts-daemon.yml", data, 0o644); err != nil {
			return err
		}
	}
	if cfg.RelayUser {
		if err := renderPostfixSASLPasswd(cfg); err != nil {
			return err
		}
		if out, err := exec.Command("postmap", "/etc/postfix/sasl_passwd").CombinedOutput(); err != nil {
			return fmt.Errorf("postmap sasl_passwd: %v\n%s", err, out)
		}
	}
	return nil
}

func nonCommentLines(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// renderPostfixSASLPasswd writes /etc/postfix/sasl_passwd from RELAYHOST /
// RELAYUSER / RELAYPASSWORD (template: "{{ RELAYHOST }} {{ RELAYUSER }}:{{ RELAYPASSWORD }}").
func renderPostfixSASLPasswd(cfg PostfixConfig) error {
	user := os.Getenv("RELAYUSER")
	pw := os.Getenv("RELAYPASSWORD")
	data := fmt.Sprintf("%s %s:%s\n", cfg.RelayHost, user, pw)
	return agent.AtomicWrite("/etc/postfix/sasl_passwd", []byte(data), 0o600)
}

// renderPostfixLogrotate writes /etc/logrotate.d/postfix.conf from
// POSTFIX_LOG_FILE, honoring an override file when present.
func renderPostfixLogrotate(cfg PostfixConfig) error {
	if data, err := os.ReadFile("/overrides/logrotate.conf"); err == nil {
		return agent.AtomicWrite("/etc/logrotate.d/postfix.conf", data, 0o644)
	}
	data := fmt.Sprintf("%s {\nweekly\nrotate 52\nnocompress\nextension log\ncreate 0644 root root\n}\n", cfg.PostfixLogFile)
	return agent.AtomicWrite("/etc/logrotate.d/postfix.conf", []byte(data), 0o644)
}

// escapePath percent-encodes a key for a URL path while preserving slashes
// (keys are quoted with safe="/" so path separators survive).
func escapePath(p string) string {
	e := url.PathEscape(p)
	return strings.ReplaceAll(e, "%2F", "/")
}
