package pgp

import (
	"strings"
	"testing"
)

func TestRoundTripEncryptDecrypt(t *testing.T) {
	pub, priv, err := GenerateKeyPair("alice@example.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	fp, err := Fingerprint(pub)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if len(fp) != 40 {
		t.Errorf("fingerprint length = %d, want 40", len(fp))
	}

	msg := "Hello, this is a secret message.\nSecond line."
	enc, err := Encrypt(pub, msg)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.Contains(enc, "BEGIN PGP MESSAGE") {
		t.Errorf("encrypted output not armored")
	}
	if strings.Contains(enc, "secret message") {
		t.Errorf("plaintext leaked into ciphertext")
	}

	dec, err := Decrypt(priv, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != msg {
		t.Errorf("round trip = %q, want %q", dec, msg)
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv, err := GenerateKeyPair("signer@example.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	msg := "signed document"
	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ok, err := Verify(pub, msg, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Error("signature should verify against the signing key")
	}
	ok, err = Verify(pub, msg+"tampered", sig)
	if err != nil || ok {
		t.Errorf("tampered message should not verify (ok=%v err=%v)", ok, err)
	}
}
