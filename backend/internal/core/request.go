package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/mail"
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

// MailDial resolves the mailbox connection for a request: the internal gateway
// account by default, or the external account named by ?account_id= when
// present. External dials carry the decrypted stored credentials so the mail
// client can connect to the aggregated server directly.
func (a *App) MailDial(c *fiber.Ctx) (mail.Dial, error) {
	user := CurrentUser(c)
	// Mailbox delegation: the caller holds a full-access grant on another
	// internal account and asks every /mail/* operation to run in that
	// mailbox context (X-Delegate-Email). The temp token is issued for the
	// owner but bound to the caller's session; the auth/email endpoint
	// accepts it through the delegation grant.
	if delegate := c.Get("X-Delegate-Email"); delegate != "" && !strings.EqualFold(delegate, user.Email) {
		var dep models.MailDelegation
		if err := a.DB.WithContext(c.Context()).
			Where("LOWER(owner_email) = LOWER(?) AND LOWER(delegate_email) = LOWER(?) AND full_access = ?", delegate, user.Email, true).
			First(&dep).Error; err != nil {
			return mail.Dial{}, errors.New("not authorized to access this mailbox")
		}
		sid := c.Cookies(a.Auth.SessionName)
		token, err := a.Auth.CreateTempToken(c.Context(), delegate, sid)
		if err != nil {
			return mail.Dial{}, err
		}
		return mail.LocalDial(delegate, token), nil
	}
	if id := c.Query("account_id"); id != "" {
		aid, err := strconv.ParseUint(id, 10, 64)
		if err != nil || aid == 0 {
			return mail.Dial{}, errors.New("invalid account_id")
		}
		var acc models.Account
		if err := a.DB.First(&acc, "id = ? AND user_email = ?", aid, user.Email).Error; err != nil {
			return mail.Dial{}, errors.New("account not found")
		}
		if !acc.Enabled {
			return mail.Dial{}, errors.New("account is disabled")
		}
		pw, err := crypto.Decrypt(a.Cfg.SecretKey, acc.PasswordEnc)
		if err != nil {
			return mail.Dial{}, errors.New("cannot unlock account credentials")
		}
		d := mail.ExternalDial(acc.Email, acc.ImapHost, acc.ImapPort, acc.ImapSecurity, acc.Username, pw)
		d.AccountID = acc.ID
		return d, nil
	}
	token, err := a.MailToken(c)
	if err != nil {
		return mail.Dial{}, err
	}
	return mail.LocalDial(user.Email, token), nil
}
