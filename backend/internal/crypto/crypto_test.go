package crypto

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	enc, err := Encrypt("dev-secret", "remote-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decrypt("dev-secret", enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "remote-pass-123" {
		t.Fatalf("round trip mismatch: %q", dec)
	}
}

func TestWrongSecretFails(t *testing.T) {
	enc, _ := Encrypt("secret-a", "value")
	if _, err := Decrypt("secret-b", enc); err == nil {
		t.Fatal("decrypt with wrong secret should fail")
	}
}
