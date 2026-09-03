package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
)

// devPrivateKeyB64 is the development signing key. It exists so local
// development, CI and trial evaluation can issue licenses without vendor
// secrets. It is committed in the open source on purpose: it only authorizes
// binaries that still embed the source-default verification key, and those
// refuse MAILEZ_LICENSE_REQUIRED=true at startup (see Load), so it can never
// unlock a production deployment. The vendor production private key is never
// committed; release signing uses MAILEZ_LICENSE_PRIVATE_KEY (see
// cmd/license).
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
