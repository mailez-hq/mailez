package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNginxRenderNotls(t *testing.T) {
	t.Setenv("HOSTNAMES", "mail.example.com")
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("POSTMASTER", "postmaster")
	t.Setenv("TLS_FLAVOR", "notls")
	t.Setenv("BACKEND_ADDRESS", "backend")
	t.Setenv("MAIL_FILTER_ADDRESS", "mail-filter")
	t.Setenv("SUBNET", "192.168.206.0/24")
	t.Setenv("RESOLVER_ADDRESS", "192.168.206.254")
	t.Setenv("API", "true")
	t.Setenv("WEBMAIL", "none")

	cfg, err := loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	if cfg.TLS != nil {
		t.Fatalf("notls flavor must yield nil TLS, got %+v", cfg.TLS)
	}

	files, err := renderNginxAll(cfg)
	if err != nil {
		t.Fatalf("renderNginxAll: %v", err)
	}
	nginx := string(files["/etc/nginx/nginx.conf"])
	for _, want := range []string{
		"resolver ", "listen 25", "listen 80",
		"location /internal {", "auth_http http://127.0.0.1:8000/auth/email",
	} {
		if !strings.Contains(nginx, want) {
			t.Errorf("nginx.conf missing %q", want)
		}
	}
	for _, banned := range []string{"listen 443", "include /etc/nginx/tls.conf", "ssl_certificate"} {
		if strings.Contains(nginx, banned) {
			t.Errorf("nginx.conf should not contain %q in notls mode", banned)
		}
	}
	if _, ok := files["/etc/nginx/tls.conf"]; ok {
		t.Errorf("tls.conf must not be rendered in notls mode")
	}

	dovecot := string(files["/etc/dovecot/proxy.conf"])
	if !strings.Contains(dovecot, "ssl = no") || strings.Contains(dovecot, "ssl = required") {
		t.Errorf("dovecot proxy TLS wrong in notls mode:\n%s", dovecot)
	}
	if !strings.Contains(dovecot, "listen = *") {
		t.Errorf("dovecot proxy should listen on IPv4 only without SUBNET6:\n%s", dovecot)
	}
	if !strings.Contains(dovecot, "port = 4190") {
		t.Errorf("managesieve 4190 missing in dovecot proxy:\n%s", dovecot)
	}

	lua := string(files["/etc/dovecot/login.lua"])
	if !strings.Contains(lua, `url = "http://backend:8080/internal/auth/email"`) {
		t.Errorf("login.lua admin address wrong:\n%s", lua)
	}
}

func TestNginxPortsDerivation(t *testing.T) {
	t.Setenv("HOSTNAMES", "mail.example.com")
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("TLS_FLAVOR", "notls")
	t.Setenv("PORTS", "25,143,110,587,4190,80")
	t.Setenv("RESOLVER_ADDRESS", "192.168.206.254")

	cfg, err := loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	if !cfg.Port143 || !cfg.Port110 || !cfg.Port587 || !cfg.Port4190 || !cfg.Port80 {
		t.Errorf("expected plain ports on, got %+v", cfg)
	}
	if cfg.Port995 {
		t.Errorf("995 requires TLS and must be off in notls mode")
	}

	t.Setenv("TLS_FLAVOR", "letsencrypt")
	t.Setenv("PORTS", "25,80,443,465,993,995,4190")
	cfg, err = loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	if !cfg.TLS443 || !cfg.TLS993 || !cfg.TLS995 || !cfg.TLS465 {
		t.Errorf("expected TLS ports on for letsencrypt, got %+v", cfg)
	}
}

func TestFirstNameserver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	os.WriteFile(path, []byte("nameserver 192.168.1.1\nnameserver 8.8.8.8\n"), 0o644)
	ns, err := firstNameserver(path)
	if err != nil {
		t.Fatalf("firstNameserver: %v", err)
	}
	if ns != "192.168.1.1" {
		t.Fatalf("got %q, want 192.168.1.1", ns)
	}

	os.WriteFile(path, []byte("nameserver fd00::1\n"), 0o644)
	ns, err = firstNameserver(path)
	if err != nil {
		t.Fatalf("firstNameserver: %v", err)
	}
	if ns != "[fd00::1]" {
		t.Fatalf("ipv6 resolver should be bracketed, got %q", ns)
	}
}

func TestNginxCertChainOutput(t *testing.T) {
	// Generate a throwaway leaf; the verify step will fail (self-signed) and
	// fall back to the raw chain, while stripCA must drop the ISRG roots.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test.example.com"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	leaf := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	dir := t.TempDir()
	full := filepath.Join(dir, "fullchain.pem")
	os.WriteFile(full, []byte(leaf+isrgRootsPEM), 0o644)

	out := filepath.Join(dir, "out.pem")
	if err := formatForNginx(full, out, true); err != nil {
		t.Fatalf("formatForNginx: %v", err)
	}
	data, _ := os.ReadFile(out)
	if strings.Count(string(data), "BEGIN CERTIFICATE") != 1 {
		t.Fatalf("stripCA should leave exactly the leaf, got:\n%s", data)
	}
}
