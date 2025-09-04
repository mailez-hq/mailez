package entitlements

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

const devKey = "MC4CAQAwBQYDK2VwBCIEIPWzNuuTjl2NrdOHPuyv5qq1symha2KyveZ1fAPwFsR/"

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	der, err := base64.StdEncoding.DecodeString(devKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		t.Fatal(err)
	}
	ed, ok := key.(ed25519.PrivateKey)
	if !ok {
		t.Fatal("not ed25519")
	}
	return ed
}

func sign(t *testing.T, payload []byte) string {
	t.Helper()
	sig := ed25519.Sign(testKey(t), payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestParseValid(t *testing.T) {
	payload := []byte(`{"v":1,"licensee":"Acme","tier":"premium","expires_at":"2027-08-28T00:00:00Z"}`)
	ent, err := Parse(sign(t, payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ent.Tier != TierPremium || ent.Licensee != "Acme" {
		t.Fatalf("unexpected: %+v", ent)
	}
}

func TestParseRejectsTampering(t *testing.T) {
	payload := []byte(`{"v":1,"tier":"standard"}`)
	env := sign(t, payload)
	parts := strings.SplitN(env, ".", 2)
	if _, err := Parse(parts[0] + "." + parts[1][:len(parts[1])-1]); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if _, err := Parse("AA" + env); err == nil {
		t.Fatal("tampered payload accepted")
	}
	if _, err := Parse(env); err != nil {
		t.Fatalf("valid cert rejected: %v", err)
	}
}

func TestLoadAndStatus(t *testing.T) {
	// No certificate -> nil manager.
	m, err := Load("", "")
	if err != nil || m != nil {
		t.Fatalf("empty load: m=%v err=%v", m, err)
	}
	// Invalid explicit certificate -> error.
	if _, err := Load("", "garbage"); err == nil {
		t.Fatal("invalid certificate accepted")
	}
	// Active certificate.
	env := sign(t, []byte(`{"v":1,"licensee":"Acme","tier":"standard","expires_at":"2027-08-28T00:00:00Z"}`))
	m, err = Load("", env)
	if err != nil || m == nil {
		t.Fatalf("load: m=%v err=%v", m, err)
	}
	st := m.Status(time.Now())
	if !st.Valid || st.Tier != TierStandard || st.Licensee != "Acme" {
		t.Fatalf("status: %+v", st)
	}
	// Expired certificate -> invalid but still loaded (informational).
	exp := sign(t, []byte(`{"v":1,"tier":"premium","expires_at":"2020-01-01T00:00:00Z"}`))
	m, err = Load("", exp)
	if err != nil || m == nil {
		t.Fatalf("load expired: err=%v", err)
	}
	if st := m.Status(time.Now()); st.Valid {
		t.Fatal("expired service reported valid")
	}
}
