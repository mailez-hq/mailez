package mail

import "net/smtp"

// PlainAuth is an smtp.Auth that always issues AUTH PLAIN, even on an
// unencrypted link. It is only for the trusted internal submission port
// (MAILEZ_TLS=off); net/smtp's own PlainAuth refuses plaintext.
type PlainAuth struct {
	identity, username, password string
}

// NewPlainAuth builds the plaintext AUTH PLAIN credentials.
func NewPlainAuth(username, password string) smtp.Auth {
	return &PlainAuth{identity: "", username: username, password: password}
}

// Start returns the AUTH PLAIN mechanism and initial response.
func (a *PlainAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

// Next has nothing further to send.
func (a *PlainAuth) Next(_ []byte, _ bool) ([]byte, error) {
	return nil, nil
}
