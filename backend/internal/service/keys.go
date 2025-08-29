package service

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
)

// publicKeyB64 is the vendor Ed25519 verification key shared with the
// license package (same keypair signs both artifacts). Production builds
// replace it via
//
//	go build -ldflags "-X mailez/backend/internal/service.publicKeyB64=<b64>"
var publicKeyB64 = "MCowBQYDK2VwAyEAp23i5ECVobMMoopuFqZdYEkCJLaCg5kYMxsU1Vtq5GA="

var publicKey = mustDecodePublicKey(publicKeyB64)

func mustDecodePublicKey(b64 string) ed25519.PublicKey {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic("service: invalid embedded public key: " + err.Error())
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		panic("service: invalid embedded public key: " + err.Error())
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		panic("service: embedded key is not Ed25519")
	}
	return ed
}
