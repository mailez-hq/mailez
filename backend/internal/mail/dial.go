package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"

	"github.com/emersion/go-imap/client"
)

// Dial describes how to reach and authenticate one mailbox. Internal accounts
// authenticate against the local gateway with a per-session temp token; external
// (aggregated) accounts dial their own server with the stored credentials.
type Dial struct {
	Email string // account address; gateway login and From identity for internal accounts
	Token string // per-session temp token (internal gateway accounts only)

	// AccountID is the stored row id of an external account (0 = internal).
	// The compose/outbox paths use it to deliver scheduled sends.
	AccountID uint

	// External server settings; empty for internal accounts.
	External bool
	Host     string
	Port     int
	Security string // none | starttls | tls
	Username string
	Password string
}

// LocalDial builds a dial for an internal gateway account.
func LocalDial(email, token string) Dial {
	return Dial{Email: email, Token: token}
}

// ExternalDial builds a dial for an external aggregated account.
func ExternalDial(email, host string, port int, security, username, password string) Dial {
	return Dial{
		Email:    email,
		External: true,
		Host:     host,
		Port:     port,
		Security: security,
		Username: username,
		Password: password,
	}
}

// With returns a copy of the client bound to dial. Every IMAP/SMTP operation on
// the copy authenticates against the dial's target instead of the shared
// gateway account, so a single stateless Client serves both the local mailbox
// and any number of aggregated external accounts.
func (c *Client) With(d Dial) Gateway {
	return &Client{
		IMAPAddr:  c.IMAPAddr,
		SMTPAddr:  c.SMTPAddr,
		SieveAddr: c.SieveAddr,
		dial:      d,
		pool:      c.pool,
	}
}

// openExternalIMAP dials an external server, upgrades the transport per its
// security policy and logs in with the stored credentials.
func openExternalIMAP(d Dial) (*client.Client, error) {
	addr := net.JoinHostPort(d.Host, strconv.Itoa(d.Port))
	tlsCfg := &tls.Config{InsecureSkipVerify: true, ServerName: d.Host}
	var cli *client.Client
	var err error
	switch d.Security {
	case "tls", "ssl", "imaps":
		cli, err = client.DialTLS(addr, tlsCfg)
	case "starttls":
		cli, err = client.Dial(addr)
		if err == nil {
			if serr := cli.StartTLS(tlsCfg); serr != nil {
				_ = cli.Logout()
				return nil, fmt.Errorf("imap starttls: %w", serr)
			}
		}
	default: // none: plaintext (for private networks / dev only)
		cli, err = client.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", addr, err)
	}
	if err := cli.Login(d.Username, d.Password); err != nil {
		_ = cli.Logout()
		return nil, fmt.Errorf("imap login: %w", err)
	}
	return cli, nil
}
