package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"mailez/backend/internal/agent"
)

const acmeAccountKey = "/certs/letsencrypt/account.key"

// acmeUser implements registration.User with a persisted account key so the
// ACME account survives container restarts (certbot kept it in its config
// dir; we keep just the key).
type acmeUser struct {
	email string
	key   crypto.PrivateKey
	reg   *registration.Resource
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.reg }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// obtainACMECertificate replaces certbot: registers (reusing the persisted
// account key), answers the HTTP-01 challenge on port 8008 (nginx proxies
// /.well-known/acme-challenge/ there) and writes fullchain.pem + privkey.pem
// in the same layout certbot produced.
func obtainACMECertificate(hostnames []string, email string, ecdsa bool) error {
	key, err := loadOrCreateAccountKey()
	if err != nil {
		return err
	}
	user := &acmeUser{email: email, key: key}

	cfg := lego.NewConfig(user)
	cfg.Certificate.KeyType = certcrypto.RSA2048
	if ecdsa {
		cfg.Certificate.KeyType = certcrypto.EC256
	}
	cfg.Certificate.Timeout = 90 * time.Second
	client, err := lego.NewClient(cfg)
	if err != nil {
		return err
	}
	_ = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "8008"))

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil && !strings.Contains(err.Error(), "already registered") {
		return fmt.Errorf("acme register: %w", err)
	}
	user.reg = reg

	request := certificate.ObtainRequest{
		Domains: hostnames,
		Bundle:  true,
	}
	res, err := client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("acme obtain: %w", err)
	}

	certName := "mailez"
	if ecdsa {
		certName = "mailez-ecdsa"
	}
	dir := filepath.Join("/certs/letsencrypt/live", certName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := agent.AtomicWrite(filepath.Join(dir, "fullchain.pem"), res.Certificate, 0o644); err != nil {
		return err
	}
	return agent.AtomicWrite(filepath.Join(dir, "privkey.pem"), res.PrivateKey, 0o600)
}

func loadOrCreateAccountKey() (crypto.PrivateKey, error) {
	if data, err := os.ReadFile(acmeAccountKey); err == nil {
		if block, _ := pem.Decode(data); block != nil {
			if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				return k, nil
			}
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.MkdirAll(filepath.Dir(acmeAccountKey), 0o755); err != nil {
		return nil, err
	}
	if err := agent.AtomicWrite(acmeAccountKey, data, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// certExpiry returns the earliest NotAfter of the leaf certificates in a
// fullchain.pem file (zero time when missing/unreadable).
func certExpiry(path string) time.Time {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	var earliest time.Time
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if earliest.IsZero() || cert.NotAfter.Before(earliest) {
			earliest = cert.NotAfter
		}
	}
	return earliest
}

// acmeLoop keeps certificates fresh. It replaces certbot: renew when the
// current certificate is missing or within 30 days of expiry.
func acmeLoop(cfg NginxConfig) {
	time.Sleep(5 * time.Second) // let nginx start
	hostnames := strings.Fields(cfgHostnames())
	if len(hostnames) == 0 {
		return
	}
	email := fmt.Sprintf("%s@%s", agent.Getenv("MAILEZ_POSTMASTER", "postmaster"), agent.Getenv("MAILEZ_DOMAIN", "example.com"))
	renew := func(ecdsa bool) {
		certName := "mailez"
		if ecdsa {
			certName = "mailez-ecdsa"
		}
		fullchain := filepath.Join("/certs/letsencrypt/live", certName, "fullchain.pem")
		if exp := certExpiry(fullchain); exp.IsZero() || time.Until(exp) < 30*24*time.Hour {
			fmt.Fprintf(os.Stderr, "nginx: obtaining %s certificate (%s)\n", certName, hostnames)
			if err := obtainACMECertificate(hostnames, email, ecdsa); err != nil {
				fmt.Fprintf(os.Stderr, "nginx: acme %s failed: %v\n", certName, err)
				return
			}
			if err := regenerateChains(); err != nil {
				fmt.Fprintf(os.Stderr, "nginx: regenerate chains after renew: %v\n", err)
			}
			_ = execNginxReload()
		}
	}
	for {
		renew(false)
		renew(true)
		time.Sleep(24 * time.Hour)
	}
}

func execNginxReload() error {
	return exec.Command("/usr/sbin/nginx", "-s", "reload").Run()
}
