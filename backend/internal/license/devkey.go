package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
)

// devPrivateKeyB64 is the DEV-ONLY signing key that pairs with the embedded
// dev public key. It exists so local development and CI can issue licenses.
// The vendor production private key is never committed; release signing uses
// MAILEZ_LICENSE_PRIVATE_KEY (see cmd/license).
var devPrivateKeyB64 = "MC4CAQAwBQYDK2VwBCIEIPWzNuuTjl2NrdOHPuyv5qq1symha2KyveZ1fAPwFsR/"

// DevPrivateKey returns the dev signing key.
func DevPrivateKey() ed25519.PrivateKey {
	der, err := base64.StdEncoding.DecodeString(devPrivateKeyB64)
	if err != nil {
		panic("license: invalid dev private key: " + err.Error())
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		panic("license: invalid dev private key: " + err.Error())
	}
	ed, ok := key.(ed25519.PrivateKey)
	if !ok {
		panic("license: dev key is not Ed25519")
	}
	return ed
}
