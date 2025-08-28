// Command license issues and inspects Mailez enterprise licenses and
// technical-service certificates.
//
//   go run ./cmd/license genkey                      # print a fresh keypair
//   go run ./cmd/license issue --out lic.lic \
//       --licensee "Acme Corp" --mailboxes 500
//   go run ./cmd/license inspect --in lic.lic
//   go run ./cmd/license service issue --out service.lic \
//       --licensee "Acme Corp" --tier premium --service-end 2027-08-28T00:00:00Z
//   go run ./cmd/license service inspect --in service.lic
//
// The signing key comes from MAILEZ_LICENSE_PRIVATE_KEY (base64 DER PKCS8)
// or --key <file>; without either the built-in DEV key is used.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"mailez/backend/internal/license"
	"mailez/backend/internal/service"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "genkey":
		genKey()
	case "issue":
		issue(os.Args[2:])
	case "inspect":
		inspect(os.Args[2:])
	case "service":
		serviceCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  license genkey
  license issue --out FILE [--licensee NAME] --mailboxes N
  license inspect --in FILE
  license service issue --out FILE [--licensee NAME] --tier standard|premium --service-end RFC3339
  license service inspect --in FILE`)
}

func genKey() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(err)
	}
	pubDER, _ := x509.MarshalPKIXPublicKey(pub)
	privDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	fmt.Printf("public : %s\n", base64.StdEncoding.EncodeToString(pubDER))
	fmt.Printf("private: %s\n", base64.StdEncoding.EncodeToString(privDER))
}

func signingKey(flagKey string) ed25519.PrivateKey {
	if flagKey != "" {
		der, err := os.ReadFile(flagKey)
		if err != nil {
			fatal(err)
		}
		key, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			fatal(err)
		}
		if ed, ok := key.(ed25519.PrivateKey); ok {
			return ed
		}
		fatal(fmt.Errorf("key file is not an Ed25519 private key"))
	}
	if env := os.Getenv("MAILEZ_LICENSE_PRIVATE_KEY"); env != "" {
		der, err := base64.StdEncoding.DecodeString(env)
		if err != nil {
			fatal(fmt.Errorf("MAILEZ_LICENSE_PRIVATE_KEY: %w", err))
		}
		key, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			fatal(err)
		}
		if ed, ok := key.(ed25519.PrivateKey); ok {
			return ed
		}
		fatal(fmt.Errorf("MAILEZ_LICENSE_PRIVATE_KEY is not an Ed25519 private key"))
	}
	fmt.Fprintln(os.Stderr, "WARNING: using the built-in DEV signing key (not for production)")
	return license.DevPrivateKey()
}

func issue(args []string) {
	fs := flag.NewFlagSet("issue", flag.ExitOnError)
	out := fs.String("out", "", "output license file")
	licensee := fs.String("licensee", "", "licensed organization")
	mailboxes := fs.Int("mailboxes", 0, "max mailboxes (0 = unlimited)")
	features := fs.String("features", "", "comma-separated feature flags")
	key := fs.String("key", "", "signing key file (PKCS8 DER)")
	_ = fs.Parse(args)

	if *mailboxes <= 0 {
		fatal(fmt.Errorf("--mailboxes must be > 0"))
	}
	lic := license.License{
		Version:      1,
		Edition:      license.EditionEnterprise,
		Licensee:     *licensee,
		MaxMailboxes: *mailboxes,
		IssuedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if *features != "" {
		for _, f := range splitCSV(*features) {
			lic.Features = append(lic.Features, f)
		}
	}
	env, err := license.Sign(lic, signingKey(*key))
	if err != nil {
		fatal(err)
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(env+"\n"), 0o600); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s\n", *out)
	} else {
		fmt.Println(env)
	}
}

func inspect(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	in := fs.String("in", "", "license file")
	_ = fs.Parse(args)
	if *in == "" {
		fatal(fmt.Errorf("--in is required"))
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	lic, err := license.Parse(string(raw))
	if err != nil {
		fatal(err)
	}
	b, _ := json.MarshalIndent(lic, "", "  ")
	fmt.Println(string(b))
}

func serviceCmd(args []string) {
	if len(args) < 1 {
		fatal(fmt.Errorf("service requires a subcommand: issue | inspect"))
	}
	switch args[0] {
	case "issue":
		serviceIssue(args[1:])
	case "inspect":
		serviceInspect(args[1:])
	default:
		fatal(fmt.Errorf("unknown service subcommand %q (want issue | inspect)", args[0]))
	}
}

func serviceIssue(args []string) {
	fs := flag.NewFlagSet("service issue", flag.ExitOnError)
	out := fs.String("out", "", "output certificate file")
	licensee := fs.String("licensee", "", "licensed organization")
	tier := fs.String("tier", "", "support tier: standard | premium")
	serviceEnd := fs.String("service-end", "", "service end RFC3339, e.g. 2027-08-28T00:00:00Z")
	features := fs.String("features", "", "comma-separated feature flags")
	key := fs.String("key", "", "signing key file (PKCS8 DER)")
	_ = fs.Parse(args)

	if *tier != service.TierStandard && *tier != service.TierPremium {
		fatal(fmt.Errorf("--tier must be %q or %q", service.TierStandard, service.TierPremium))
	}
	if *serviceEnd == "" {
		fatal(fmt.Errorf("--service-end is required (RFC3339)"))
	}
	t, err := time.Parse(time.RFC3339, *serviceEnd)
	if err != nil {
		fatal(fmt.Errorf("--service-end: %w", err))
	}
	ent := service.Entitlement{
		Version:   1,
		Licensee:  *licensee,
		Tier:      *tier,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: t.UTC().Format(time.RFC3339),
	}
	if *features != "" {
		for _, f := range splitCSV(*features) {
			ent.Features = append(ent.Features, f)
		}
	}
	env, err := service.Sign(ent, signingKey(*key))
	if err != nil {
		fatal(err)
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(env+"\n"), 0o600); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s\n", *out)
	} else {
		fmt.Println(env)
	}
}

func serviceInspect(args []string) {
	fs := flag.NewFlagSet("service inspect", flag.ExitOnError)
	in := fs.String("in", "", "certificate file")
	_ = fs.Parse(args)
	if *in == "" {
		fatal(fmt.Errorf("--in is required"))
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	ent, err := service.Parse(string(raw))
	if err != nil {
		fatal(err)
	}
	b, _ := json.MarshalIndent(ent, "", "  ")
	fmt.Println(string(b))
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "license:", err)
	os.Exit(1)
}
