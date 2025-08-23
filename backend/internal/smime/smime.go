package smime

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	pkcs7 "go.mozilla.org/pkcs7"
	"golang.org/x/crypto/pkcs12"
)

func init() {
	// Prefer modern authenticated encryption over the 3DES default.
	pkcs7.ContentEncryptionAlgorithm = pkcs7.EncryptionAlgorithmAES256GCM
}

// CertInfo describes a parsed X.509 certificate in terms the UI can show.
type CertInfo struct {
	Email       string    `json:"email"`
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	Serial      string    `json:"serial"`
	Fingerprint string    `json:"fingerprint"` // SHA-256, uppercase hex
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
}

// ParseCertificate parses the first certificate found in PEM data.
func ParseCertificate(pemData string) (*x509.Certificate, CertInfo, error) {
	rest := []byte(pemData)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, CertInfo{}, errors.New("smime: no certificate found in PEM data")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, CertInfo{}, fmt.Errorf("smime: parse certificate: %w", err)
		}
		return cert, InfoOf(cert), nil
	}
}

// InfoOf extracts the display fields of a certificate.
func InfoOf(cert *x509.Certificate) CertInfo {
	fp := sha256.Sum256(cert.Raw)
	return CertInfo{
		Email:       emailOf(cert),
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		Serial:      cert.SerialNumber.String(),
		Fingerprint: strings.ToUpper(hex.EncodeToString(fp[:])),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
	}
}

// CertToPEM serialises a certificate to PEM text.
func CertToPEM(cert *x509.Certificate) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
}

// PrivateKeyToPEM serialises a private key to PKCS#8 PEM (the most portable
// form; EC and RSA keys alike round-trip through x509.ParsePKCS8PrivateKey).
func PrivateKeyToPEM(key crypto.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("smime: marshal private key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// KeyMatches reports whether a PEM private key corresponds to the certificate's
// public key, guarding against importing a mismatched pair.
func KeyMatches(cert *x509.Certificate, privKeyPEM string) error {
	key, err := ParsePrivateKey(privKeyPEM)
	if err != nil {
		return err
	}
	pub, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return fmt.Errorf("smime: marshal cert public key: %w", err)
	}
	keyPub, err := x509.MarshalPKIXPublicKey(pubKeyOf(key))
	if err != nil {
		return fmt.Errorf("smime: marshal key public half: %w", err)
	}
	if string(pub) != string(keyPub) {
		return errors.New("smime: private key does not match the certificate")
	}
	return nil
}

func pubKeyOf(key crypto.PrivateKey) crypto.PublicKey {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func emailOf(cert *x509.Certificate) string {
	if len(cert.EmailAddresses) > 0 {
		return strings.ToLower(strings.TrimSpace(cert.EmailAddresses[0]))
	}
	cn := strings.ToLower(strings.TrimSpace(cert.Subject.CommonName))
	if strings.Contains(cn, "@") {
		return cn
	}
	return ""
}

// ParsePrivateKey parses a PEM private key (PKCS#1, SEC1 or PKCS#8). Encrypted
// keys are rejected; the caller is expected to supply a decrypted key.
func ParsePrivateKey(pemData string) (crypto.PrivateKey, error) {
	rest := []byte(pemData)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("smime: no private key found in PEM data")
		}
		switch block.Type {
		case "RSA PRIVATE KEY":
			return x509.ParsePKCS1PrivateKey(block.Bytes)
		case "EC PRIVATE KEY":
			return x509.ParseECPrivateKey(block.Bytes)
		case "PRIVATE KEY":
			return x509.ParsePKCS8PrivateKey(block.Bytes)
		case "ENCRYPTED PRIVATE KEY":
			return nil, errors.New("smime: encrypted private keys are not supported; decrypt the key first")
		}
	}
}

// ParsePKCS12 decodes a .p12/.pfx bundle and returns its leaf certificate and
// private key. Password may be empty for unencrypted bundles.
func ParsePKCS12(data []byte, password string) (*x509.Certificate, crypto.PrivateKey, error) {
	key, cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, nil, fmt.Errorf("smime: pkcs12 decode: %w", err)
	}
	return cert, key, nil
}

// Encrypt seals plaintext into a CMS enveloped-data message for the recipient
// certificate and returns it base64-encoded.
func Encrypt(recipientCertPEM, plaintext string) (string, error) {
	cert, _, err := ParseCertificate(recipientCertPEM)
	if err != nil {
		return "", err
	}
	if err := usableForEncryption(cert); err != nil {
		return "", err
	}
	enveloped, err := pkcs7.Encrypt([]byte(plaintext), []*x509.Certificate{cert})
	if err != nil {
		return "", fmt.Errorf("smime encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(enveloped), nil
}

// Decrypt opens a base64-encoded CMS enveloped-data message with the given
// PEM private key and its matching certificate.
func Decrypt(privKeyPEM, certPEM, ciphertextB64 string) (string, error) {
	key, err := ParsePrivateKey(privKeyPEM)
	if err != nil {
		return "", err
	}
	cert, _, err := ParseCertificate(certPEM)
	if err != nil {
		return "", err
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertextB64))
	if err != nil {
		return "", errors.New("smime: invalid base64 ciphertext")
	}
	p7, err := pkcs7.Parse(der)
	if err != nil {
		return "", fmt.Errorf("smime parse: %w", err)
	}
	plain, err := p7.Decrypt(cert, key)
	if err != nil {
		return "", fmt.Errorf("smime decrypt: %w", err)
	}
	return string(plain), nil
}

// Sign produces a CMS signed-data message over plaintext using the given
// certificate and private key, returning it base64-encoded.
func Sign(certPEM, privKeyPEM, plaintext string) (string, error) {
	key, err := ParsePrivateKey(privKeyPEM)
	if err != nil {
		return "", err
	}
	cert, _, err := ParseCertificate(certPEM)
	if err != nil {
		return "", err
	}
	sd, err := pkcs7.NewSignedData([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("smime sign: %w", err)
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		return "", fmt.Errorf("smime sign: %w", err)
	}
	signed, err := sd.Finish()
	if err != nil {
		return "", fmt.Errorf("smime sign finish: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signed), nil
}

// Verify checks a base64-encoded CMS signed-data message against a signer
// certificate. It reports whether the signature is valid and, when valid, the
// embedded content that was signed.
func Verify(signerCertPEM, signedB64 string) (valid bool, content string, err error) {
	signer, _, err := ParseCertificate(signerCertPEM)
	if err != nil {
		return false, "", err
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signedB64))
	if err != nil {
		return false, "", errors.New("smime: invalid base64 signature")
	}
	p7, err := pkcs7.Parse(der)
	if err != nil {
		return false, "", fmt.Errorf("smime parse: %w", err)
	}
	if err := p7.Verify(); err != nil {
		return false, "", nil // signature invalid or not signed by the cert
	}
	// Confirm the signer certificate is among the embedded certificates.
	if !containsCert(p7.Certificates, signer) {
		return false, "", nil
	}
	return true, string(p7.Content), nil
}

func containsCert(certs []*x509.Certificate, target *x509.Certificate) bool {
	for _, c := range certs {
		if c.Equal(target) {
			return true
		}
	}
	return false
}

func usableForEncryption(cert *x509.Certificate) error {
	if cert.PublicKeyAlgorithm != x509.RSA {
		return errors.New("smime: recipient certificate must use an RSA key")
	}
	if len(cert.ExtKeyUsage) > 0 {
		for _, u := range cert.ExtKeyUsage {
			if u == x509.ExtKeyUsageEmailProtection {
				return nil
			}
		}
		return errors.New("smime: recipient certificate is not marked for email protection")
	}
	return nil
}
