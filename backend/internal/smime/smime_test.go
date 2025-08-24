package smime

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// newTestIdentity builds a self-signed S/MIME certificate + private key for an
// address, suitable for round-trip tests.
func newTestIdentity(t *testing.T, email string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:   big.NewInt(1),
		Subject:        pkix.Name{CommonName: email, Organization: []string{"mailez test"}},
		NotBefore:      time.Now().Add(-time.Hour),
		NotAfter:       time.Now().Add(24 * time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
		EmailAddresses: []string{email},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	keyPEM, err = PrivateKeyToPEM(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return CertToPEM(cert), keyPEM
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	recipientCert, recipientKey := newTestIdentity(t, "bob@example.com")
	ct, err := Encrypt(recipientCert, "hello smime")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "" {
		t.Fatal("empty ciphertext")
	}
	pt, err := Decrypt(recipientKey, recipientCert, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if pt != "hello smime" {
		t.Fatalf("round trip mismatch: %q", pt)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	recipientCert, _ := newTestIdentity(t, "bob@example.com")
	otherCert, otherKey := newTestIdentity(t, "mallory@example.com")
	ct, err := Encrypt(recipientCert, "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := Decrypt(otherKey, otherCert, ct); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	signerCert, signerKey := newTestIdentity(t, "alice@example.com")
	sig, err := Sign(signerCert, signerKey, "signed text")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	valid, content, err := Verify(signerCert, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid")
	}
	if content != "signed text" {
		t.Fatalf("content mismatch: %q", content)
	}
}

func TestVerifyWrongCert(t *testing.T) {
	signerCert, signerKey := newTestIdentity(t, "alice@example.com")
	otherCert, _ := newTestIdentity(t, "mallory@example.com")
	sig, err := Sign(signerCert, signerKey, "signed text")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	valid, _, err := Verify(otherCert, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if valid {
		t.Fatal("signature should not verify against a different cert")
	}
}

func TestKeyMatches(t *testing.T) {
	cert, key := newTestIdentity(t, "bob@example.com")
	otherCert, otherKey := newTestIdentity(t, "mallory@example.com")

	if err := KeyMatches(parseCert(t, cert), key); err != nil {
		t.Fatalf("matching key should pass: %v", err)
	}
	if err := KeyMatches(parseCert(t, cert), otherKey); err == nil {
		t.Fatal("mismatched key should fail")
	}
	if err := KeyMatches(parseCert(t, otherCert), key); err == nil {
		t.Fatal("mismatched cert should fail")
	}
}

func parseCert(t *testing.T, pem string) *x509.Certificate {
	t.Helper()
	cert, _, err := ParseCertificate(pem)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}
