package stack

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"strings"
)

// srsCodec implements Sender Rewriting Scheme (SRS0) with an HMAC-SHA1
// signature, matching the the mail stack's SRS address format:
//
//	HASH.SRS0=TT=domain=localpart@srs_domain
//
// Used to rewrite envelope addresses on forwarded mail so bounces return to
// the original sender.
type srsCodec struct {
	secret []byte
}

func newSRSCodec(secret string) *srsCodec {
	return &srsCodec{secret: []byte(secret)}
}

func (s *srsCodec) forward(localpart, domain, srsDomain string) string {
	tt := randomTokenString(2)
	body := "SRS0=" + tt + "=" + domain + "=" + localpart
	return s.hash(body) + "." + body + "@" + srsDomain
}

// reverse restores localpart@domain from a SRS0 address, verifying the
// signature first.
func (s *srsCodec) reverse(addr string) (string, bool) {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return "", false
	}
	user, _ := addr[:at], addr[at+1:]
	dot := strings.Index(user, ".")
	if dot < 0 {
		return "", false
	}
	hash, body := user[:dot], user[dot+1:]
	parts := strings.Split(body, "=")
	if len(parts) != 4 || parts[0] != "SRS0" {
		return "", false
	}
	if s.hash(body) != hash {
		return "", false
	}
	return parts[3] + "@" + parts[2], true
}

func (s *srsCodec) hash(body string) string {
	mac := hmac.New(sha1.New, s.secret)
	mac.Write([]byte(body))
	return strings.TrimRight(base32.StdEncoding.EncodeToString(mac.Sum(nil)[0:3]), "=")
}

func isSRSAddress(addr string) bool {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	user := addr[:at]
	dot := strings.Index(user, ".")
	if dot < 0 {
		return false
	}
	return strings.HasPrefix(user[dot+1:], "SRS0=")
}

// randomTokenString returns n random bytes hex-encoded.
func randomTokenString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", 2*n)
	}
	const hex = "0123456789abcdef"
	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(hex[v>>4])
		sb.WriteByte(hex[v&0x0f])
	}
	return sb.String()
}
