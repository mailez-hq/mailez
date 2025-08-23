package totp

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateAndValidate(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(secret) < 20 {
		t.Errorf("secret too short: %q", secret)
	}
	now := time.Now()
	code, err := Code(secret, now)
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("code length = %d", len(code))
	}
	if !Valid(secret, code, now) {
		t.Error("valid code rejected")
	}
	if Valid(secret, "000000", now) {
		t.Error("bogus code accepted")
	}
	// A code one step old should still verify (clock drift tolerance).
	prev, _ := Code(secret, now.Add(-time.Duration(stepSeconds)*time.Second))
	if !Valid(secret, prev, now) {
		t.Error("previous-step code rejected")
	}
}

func TestCodeIsDeterministic(t *testing.T) {
	secret := strings.Repeat("A", 26) // valid base32
	a, err := Code(secret, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	b, _ := Code(secret, time.Unix(1_700_000_000, 0))
	if a != b {
		t.Errorf("same time produced different codes")
	}
}
