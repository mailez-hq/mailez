// Package agent provides the shared runtime for the mailez mail
// container agents: typed env parsing, atomic config writes, and child
// process supervision with graceful signal forwarding.
package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

// Getenv returns the value of key, or def when unset or empty.
func Getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// RequireEnv returns the value of key, erroring when unset or empty.
func RequireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return v, nil
}

// CIDR returns the validated CIDR string for key.
func CIDR(key string) (string, error) {
	v, err := RequireEnv(key)
	if err != nil {
		return "", err
	}
	if _, _, err := net.ParseCIDR(v); err != nil {
		return "", fmt.Errorf("%s=%q is not a valid CIDR: %w", key, v, err)
	}
	return v, nil
}

// AtomicWrite writes data to path via a temp file + rename, so a failed
// render never leaves a partially-written config behind.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// RunChild starts args[0] with the rest as arguments, forwards its stdio to
// the agent's, and relays SIGTERM/SIGINT so the child can shut down
// gracefully. It returns when the child exits or ctx is cancelled.
func RunChild(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("RunChild: no command")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", args[0], err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Signal(syscall.SIGTERM)
		case s := <-sig:
			_ = cmd.Process.Signal(s)
		case <-done:
		}
	}()
	err := cmd.Wait()
	close(done)
	if err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	return nil
}
