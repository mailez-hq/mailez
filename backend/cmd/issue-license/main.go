//go:build mailez_ee

// issue-license mints a signed license file for a customer deployment.
// Development/evaluation builds sign with the built-in dev key (valid only
// against dev-key binaries); production issuance uses the vendor private
// key via -keyfile (base64 PKCS#8 Ed25519), matching the public key baked
// into release images with the MAILEZ_LICENSE_PUBKEY build argument.
//
// Usage:
//
//	go run -tags mailez_ee ./cmd/issue-license \
//	  -licensee "Acme Ltd." -mailboxes 25 -out license.lic
//	go run -tags mailez_ee ./cmd/issue-license \
//	  -licensee "Acme Ltd." -mailboxes 25 -keyfile vendor.key -out license.lic
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"mailez/backend/internal/ee/license"
)

func main() {
	edition := flag.String("edition", "enterprise", "license edition")
	licensee := flag.String("licensee", "", "licensee (customer) name")
	mailboxes := flag.Int("mailboxes", 0, "max mailboxes (0 = unlimited)")
	features := flag.String("features", "", "comma-separated feature flags")
	out := flag.String("out", "license.lic", "output license file")
	keyfile := flag.String("keyfile", "", "base64 PKCS#8 Ed25519 private key (default: built-in dev key)")
	flag.Parse()
	if *licensee == "" {
		fmt.Println("issue-license: -licensee is required")
		os.Exit(2)
	}

	signingKey := license.DevPrivateKey()
	if *keyfile != "" {
		raw, err := os.ReadFile(*keyfile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "issue-license: read keyfile:", err)
			os.Exit(1)
		}
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "issue-license: decode keyfile:", err)
			os.Exit(1)
		}
		k, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			fmt.Fprintln(os.Stderr, "issue-license: parse keyfile:", err)
			os.Exit(1)
		}
		key, ok := k.(ed25519.PrivateKey)
		if !ok {
			fmt.Fprintln(os.Stderr, "issue-license: keyfile is not an Ed25519 private key")
			os.Exit(1)
		}
		signingKey = key
	}

	lic := license.License{
		Version:      1,
		Edition:      *edition,
		Licensee:     *licensee,
		MaxMailboxes: *mailboxes,
		IssuedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if *features != "" {
		lic.Features = strings.Split(*features, ",")
	}
	tok, err := license.Sign(lic, signingKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "issue-license: sign:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(tok), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "issue-license: write:", err)
		os.Exit(1)
	}
	fmt.Printf("issued -> %s (edition=%s licensee=%q max_mailboxes=%d)\n",
		*out, *edition, *licensee, *mailboxes)
}
