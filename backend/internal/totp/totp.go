// Package totp implements RFC 6238 TOTP using only the standard library, so
// 2FA needs no third-party dependency.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const stepSeconds = 30

// GenerateSecret returns a new 160-bit base32 secret (no padding).
func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="), nil
}

// Code returns the 6-digit code for a secret at the given time.
func Code(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / stepSeconds)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%06d", bin%1000000), nil
}

// Valid checks a code within a ±1 step window (clock drift tolerance).
func Valid(secret, code string, t time.Time) bool {
	for i := -1; i <= 1; i++ {
		c, err := Code(secret, t.Add(time.Duration(i)*stepSeconds*time.Second))
		if err == nil && c == code {
			return true
		}
	}
	return false
}

// URI builds the otpauth:// provisioning URI for authenticator apps.
func URI(issuer, account, secret string) string {
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=%d",
		issuer, account, secret, issuer, stepSeconds,
	)
}
