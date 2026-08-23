package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// StringList accepts either a JSON string ("a@x, b@y") or an array of
// addresses, keeping the wire format tolerant for older clients.
type StringList []string

func (s *StringList) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*s = SplitAddresses(one)
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	var out []string
	for _, v := range many {
		out = append(out, SplitAddresses(v)...)
	}
	*s = out
	return nil
}

// SplitAddresses splits a comma-separated address list, trimming whitespace
// and dropping empty entries.
func SplitAddresses(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NewAppToken generates a 32-char hex secret, matching auth.IsAppToken.
func NewAppToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// MailToken issues a temp token for the current session so the IMAP gateway
// can authenticate without ever touching the user's password.
func (a *App) MailToken(c *fiber.Ctx) (string, error) {
	user := CurrentUser(c)
	sid := c.Cookies(a.Auth.SessionName)
	return a.Auth.CreateTempToken(c.Context(), user.Email, sid)
}
