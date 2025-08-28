package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
)

// publicKeyB64 is the base64 DER (SubjectPublicKeyInfo) of the vendor
// Ed25519 verification key embedded in the binary.
//
// The value below is the Mailez HQ DEV key. Production builds replace it via
//   go build -ldflags "-X mailez/backend/internal/license.publicKeyB64=<b64>"
// The matching private key lives only in the vendor's secret store and never
// ships in source, images or docs.
var publicKeyB64 = "MCowBQYDK2VwAyEAp23i5ECVobMMoopuFqZdYEkCJLaCg5kYMxsU1Vtq5GA="

var publicKey = mustDecodePublicKey(publicKeyB64)

func mustDecodePublicKey(b64 string) ed25519.PublicKey {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic("license: invalid embedded public key: " + err.Error())
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		panic("license: invalid embedded public key: " + err.Error())
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		panic("license: embedded key is not Ed25519")
	}
	return ed
}
