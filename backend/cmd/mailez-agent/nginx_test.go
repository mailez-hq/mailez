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
	t.Setenv("MAILEZ_HOSTNAMES", "mail.example.com")
	t.Setenv("MAILEZ_DOMAIN", "example.com")
	t.Setenv("MAILEZ_POSTMASTER", "postmaster")
	t.Setenv("MAILEZ_TLS", "off")
	t.Setenv("MAILEZ_BACKEND_ADDRESS", "backend")
	t.Setenv("MAIL_FILTER_ADDRESS", "mail-filter")
	t.Setenv("MAILEZ_SUBNET", "192.168.206.0/24")
	t.Setenv("MAILEZ_RESOLVER_ADDRESS", "192.168.206.254")
	t.Setenv("MAILEZ_API", "true")

	cfg, err := loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	if cfg.TLS != nil {
		t.Fatalf("off flavor must yield nil TLS, got %+v", cfg.TLS)
	}

	files, err := renderNginxAll(cfg)
	if err != nil {
		t.Fatalf("renderNginxAll: %v", err)
	}
	nginx := string(files["/etc/nginx/nginx.conf"])
	for _, want := range []string{
		"resolver ", "listen 80",
		"location /stack {",
	} {
		if !strings.Contains(nginx, want) {
			t.Errorf("nginx.conf missing %q", want)
		}
	}
	for _, banned := range []string{"listen 443", "include /etc/nginx/tls.conf", "ssl_certificate"} {
		if strings.Contains(nginx, banned) {
			t.Errorf("nginx.conf should not contain %q in off mode", banned)
		}
	}
	if _, ok := files["/etc/nginx/tls.conf"]; ok {
		t.Errorf("tls.conf must not be rendered in off mode")
	}
}

func TestNginxRenderMailezineEngine(t *testing.T) {
	t.Setenv("MAILEZ_HOSTNAMES", "mail.example.com")
	t.Setenv("MAILEZ_DOMAIN", "example.com")
	t.Setenv("MAILEZ_POSTMASTER", "postmaster")
	t.Setenv("MAILEZ_TLS", "off")
	t.Setenv("MAILEZ_BACKEND_ADDRESS", "backend")
	t.Setenv("MAILEZ_SUBNET", "192.168.206.0/24")
	t.Setenv("MAILEZ_RESOLVER_ADDRESS", "192.168.206.254")
	t.Setenv("MAILEZ_ENGINE", "mailezine")
	t.Setenv("MAILEZINE_ADDRESS", "mailezine")

	cfg, err := loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	files, err := renderNginxAll(cfg)
	if err != nil {
		t.Fatalf("renderNginxAll: %v", err)
	}
	// mailezine mode renders the HTTP/ACME nginx config only: no IMAP login
	// proxy sidecars, no mail{} proxy block (the engine publishes mail ports).
	nginx := string(files["/etc/nginx/nginx.conf"])
	for _, want := range []string{
		"resolver ", "listen 80", "location /stack {", "proxy_pass http://$backend",
		"listen 127.0.0.1:10204",
	} {
		if !strings.Contains(nginx, want) {
			t.Errorf("nginx.conf missing %q:\n%s", want, nginx)
		}
	}
	if strings.Contains(nginx, "mail {") || strings.Contains(nginx, "listen 25") ||
		strings.Contains(nginx, "auth_http") {
		t.Errorf("nginx.conf must not contain the mail proxy in mailezine mode:\n%s", nginx)
	}
	if _, ok := files["/etc/dovecot/proxy.conf"]; ok {
		t.Errorf("legacy IMAP proxy must not be rendered in mailezine mode")
	}
	if _, ok := files["/etc/dovecot/login.lua"]; ok {
		t.Errorf("legacy login.lua must not be rendered in mailezine mode")
	}
	if _, ok := files["/etc/caddy/Caddyfile"]; ok {
		t.Errorf("Caddyfile must not be rendered anymore")
	}
	if _, ok := files["/etc/nginx/proxy.conf"]; !ok {
		t.Errorf("proxy.conf must be rendered in mailezine mode")
	}
}

func TestNginxPortsDerivation(t *testing.T) {
	t.Setenv("MAILEZ_HOSTNAMES", "mail.example.com")
	t.Setenv("MAILEZ_DOMAIN", "example.com")
	t.Setenv("MAILEZ_TLS", "off")
	t.Setenv("MAILEZ_PORTS", "80,443,25,143")
	t.Setenv("MAILEZ_RESOLVER_ADDRESS", "192.168.206.254")

	cfg, err := loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	if !cfg.Port80 {
		t.Errorf("expected port 80 on, got %+v", cfg)
	}
	if cfg.TLS443 {
		t.Errorf("443 requires TLS and must be off in off mode")
	}

	t.Setenv("MAILEZ_TLS", "letsencrypt")
	t.Setenv("MAILEZ_PORTS", "80,443")
	cfg, err = loadNginxConfig()
	if err != nil {
		t.Fatalf("loadNginxConfig: %v", err)
	}
	if !cfg.Port80 || !cfg.TLS443 {
		t.Errorf("expected 80 + TLS 443 for letsencrypt, got %+v", cfg)
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
