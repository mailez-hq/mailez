package pgp

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// messageArmorType is the armor header type for an OpenPGP message.
const messageArmorType = "PGP MESSAGE"

// GenerateKeyPair creates a new RSA-2048 key pair and returns both halves as
// ASCII-armored OpenPGP keys. The private key is stored unencrypted; callers
// that persist it must encrypt it at rest.
func GenerateKeyPair(email string) (publicKey, privateKey string, err error) {
	entity, err := openpgp.NewEntity("", "", email, &packet.Config{
		RSABits: 2048,
		Time:    time.Now,
	})
	if err != nil {
		return "", "", fmt.Errorf("pgp generate: %w", err)
	}

	var pubBuf bytes.Buffer
	pubWriter, err := armor.Encode(&pubBuf, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", "", fmt.Errorf("pgp armor pub: %w", err)
	}
	if err := entity.Serialize(pubWriter); err != nil {
		return "", "", fmt.Errorf("pgp serialize pub: %w", err)
	}
	if err := pubWriter.Close(); err != nil {
		return "", "", fmt.Errorf("pgp close pub: %w", err)
	}

	var privBuf bytes.Buffer
	privWriter, err := armor.Encode(&privBuf, openpgp.PrivateKeyType, nil)
	if err != nil {
		return "", "", fmt.Errorf("pgp armor priv: %w", err)
	}
	if err := entity.SerializePrivate(privWriter, nil); err != nil {
		return "", "", fmt.Errorf("pgp serialize priv: %w", err)
	}
	if err := privWriter.Close(); err != nil {
		return "", "", fmt.Errorf("pgp close priv: %w", err)
	}
	return pubBuf.String(), privBuf.String(), nil
}

// Fingerprint returns the hex fingerprint of an armored public key.
func Fingerprint(publicKeyArmored string) (string, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(publicKeyArmored))
	if err != nil {
		return "", fmt.Errorf("pgp read key: %w", err)
	}
	if len(ring) == 0 || ring[0].PrimaryKey == nil {
		return "", errors.New("pgp key: no primary key")
	}
	return strings.ToUpper(hex.EncodeToString(ring[0].PrimaryKey.Fingerprint)), nil
}

// KeyInfo describes a parsed armored public key: its fingerprint and the email
// addresses declared on it.
type KeyInfo struct {
	Fingerprint string
	Emails      []string
}

// ParsePublicKey reads an armored public key and extracts its fingerprint and
// declared identity addresses, for storing in a user's keyring.
func ParsePublicKey(armored string) (KeyInfo, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armored))
	if err != nil {
		return KeyInfo{}, fmt.Errorf("pgp read key: %w", err)
	}
	if len(ring) == 0 || ring[0].PrimaryKey == nil {
		return KeyInfo{}, errors.New("pgp key: no primary key")
	}
	info := KeyInfo{
		Fingerprint: strings.ToUpper(hex.EncodeToString(ring[0].PrimaryKey.Fingerprint)),
	}
	seen := map[string]bool{}
	for _, id := range ring[0].Identities {
		e := strings.ToLower(strings.TrimSpace(id.UserId.Email))
		if e != "" && !seen[e] {
			seen[e] = true
			info.Emails = append(info.Emails, e)
		}
	}
	return info, nil
}

// Encrypt seals plaintext for the given armored public key and returns an
// armored OpenPGP message.
func Encrypt(publicKeyArmored, plaintext string) (string, error) {
	recipient, err := openpgp.ReadArmoredKeyRing(strings.NewReader(publicKeyArmored))
	if err != nil {
		return "", fmt.Errorf("pgp read recipient: %w", err)
	}
	var msg bytes.Buffer
	w, err := openpgp.Encrypt(&msg, recipient, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("pgp encrypt: %w", err)
	}
	if _, err := io.WriteString(w, plaintext); err != nil {
		return "", fmt.Errorf("pgp encrypt write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("pgp encrypt close: %w", err)
	}

	var out bytes.Buffer
	aw, err := armor.Encode(&out, messageArmorType, nil)
	if err != nil {
		return "", fmt.Errorf("pgp armor msg: %w", err)
	}
	if _, err := aw.Write(msg.Bytes()); err != nil {
		return "", fmt.Errorf("pgp armor write: %w", err)
	}
	if err := aw.Close(); err != nil {
		return "", fmt.Errorf("pgp armor close: %w", err)
	}
	return out.String(), nil
}

// Decrypt opens an armored OpenPGP message with the given armored private key.
func Decrypt(privateKeyArmored, ciphertextArmored string) (string, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(privateKeyArmored))
	if err != nil {
		return "", fmt.Errorf("pgp read key: %w", err)
	}
	block, err := armor.Decode(strings.NewReader(ciphertextArmored))
	if err != nil {
		return "", fmt.Errorf("pgp decode message: %w", err)
	}
	md, err := openpgp.ReadMessage(block.Body, ring, nil, nil)
	if err != nil {
		return "", fmt.Errorf("pgp decrypt: %w", err)
	}
	data, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return "", fmt.Errorf("pgp decrypt read: %w", err)
	}
	return string(data), nil
}

// Sign produces an armored detached signature over plaintext.
func Sign(privateKeyArmored, plaintext string) (string, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(privateKeyArmored))
	if err != nil {
		return "", fmt.Errorf("pgp read signer: %w", err)
	}
	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sig, ring[0], strings.NewReader(plaintext), nil); err != nil {
		return "", fmt.Errorf("pgp sign: %w", err)
	}
	return sig.String(), nil
}

// Verify checks an armored detached signature against a public key.
func Verify(publicKeyArmored, plaintext, signatureArmored string) (bool, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(publicKeyArmored))
	if err != nil {
		return false, fmt.Errorf("pgp read verifier: %w", err)
	}
	block, err := armor.Decode(strings.NewReader(signatureArmored))
	if err != nil {
		return false, fmt.Errorf("pgp decode sig: %w", err)
	}
	_, err = openpgp.CheckDetachedSignature(ring, strings.NewReader(plaintext), block.Body, nil)
	if err != nil {
		return false, nil // signature invalid or not from the given key
	}
	return true, nil
}
