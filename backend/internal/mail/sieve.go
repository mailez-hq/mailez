package mail

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SieveScript is one script reported by LISTSCRIPTS.
type SieveScript struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// sieve is a minimal RFC 5804 ManageSieve client, scoped to the operations the
// webmail filter editor needs. Every method opens its own connection
// authenticated with the per-session temp token, like the IMAP gateway.
type sieve struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func (c *Client) openSieve(email, token string) (*sieve, error) {
	conn, err := net.DialTimeout("tcp", c.SieveAddr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("sieve dial: %w", err)
	}
	s := &sieve{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	if err := s.handshake(email, token); err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

func (s *sieve) close() {
	_ = s.writeLine("LOGOUT")
	_ = s.conn.Close()
}

func (s *sieve) handshake(email, token string) error {
	greeting, err := s.readLine()
	if err != nil {
		return fmt.Errorf("sieve greeting: %w", err)
	}
	if statusOf(greeting) != "OK" {
		return fmt.Errorf("sieve greeting: %s", greeting)
	}
	// Try STARTTLS; the internal link may be plaintext behind the front proxy,
	// in which case we continue unencrypted.
	if err := s.startTLS(); err == nil {
		// upgraded
	}
	auth := base64.StdEncoding.EncodeToString([]byte("\x00" + email + "\x00" + token))
	status, _, err := s.do(`AUTHENTICATE "PLAIN" ` + auth)
	if err != nil {
		return err
	}
	if status != "OK" {
		return fmt.Errorf("sieve auth: %s", status)
	}
	return nil
}

func (s *sieve) startTLS() error {
	status, _, err := s.do("STARTTLS")
	if err != nil || status != "OK" {
		return fmt.Errorf("sieve starttls: %s", status)
	}
	tlsConn := tls.Client(s.conn, &tls.Config{InsecureSkipVerify: true}) // internal link
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("sieve tls handshake: %w", err)
	}
	s.conn = tlsConn
	s.r = bufio.NewReader(tlsConn)
	s.w = bufio.NewWriter(tlsConn)
	return nil
}

func (s *sieve) writeLine(line string) error {
	if _, err := s.w.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return s.w.Flush()
}

func (s *sieve) readLine() (string, error) {
	line, err := s.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// do sends a command and reads the full response: status line plus any
// preceding content lines and literals.
func (s *sieve) do(cmd string) (status string, content []string, err error) {
	if err := s.writeLine(cmd); err != nil {
		return "", nil, fmt.Errorf("sieve write: %w", err)
	}
	return s.readResponse()
}

var literalRe = regexp.MustCompile(`^\{(\d+)\+?\}$`)

func (s *sieve) readResponse() (status string, content []string, err error) {
	for {
		line, err := s.readLine()
		if err != nil {
			return "", nil, fmt.Errorf("sieve read: %w", err)
		}
		if st := statusOf(line); st != "" {
			return st, content, nil
		}
		if m := literalRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			buf := make([]byte, n)
			if _, err := io.ReadFull(s.r, buf); err != nil {
				return "", nil, fmt.Errorf("sieve literal: %w", err)
			}
			_, _ = s.readLine() // trailing CRLF after the literal
			content = append(content, string(buf))
			continue
		}
		content = append(content, line)
	}
}

func statusOf(line string) string {
	if line == "OK" || strings.HasPrefix(line, "OK ") {
		return "OK"
	}
	if line == "NO" || strings.HasPrefix(line, "NO ") || strings.HasPrefix(line, "NO\t") {
		return "NO"
	}
	if line == "BYE" || strings.HasPrefix(line, "BYE ") {
		return "BYE"
	}
	return ""
}

// SieveListScripts returns the user's scripts with the active flag.
func (c *Client) SieveListScripts(email, token string) ([]SieveScript, error) {
	s, err := c.openSieve(email, token)
	if err != nil {
		return nil, err
	}
	defer s.close()
	status, content, err := s.do("LISTSCRIPTS")
	if err != nil {
		return nil, err
	}
	if status != "OK" {
		return nil, fmt.Errorf("sieve list: %s", status)
	}
	var out []SieveScript
	for _, line := range content {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		name := strings.Trim(fields[0], `"`)
		active := len(fields) > 1 && fields[1] == "ACTIVE"
		out = append(out, SieveScript{Name: name, Active: active})
	}
	return out, nil
}

// SieveGetScript returns the raw content of a script.
func (c *Client) SieveGetScript(email, token, name string) (string, error) {
	s, err := c.openSieve(email, token)
	if err != nil {
		return "", err
	}
	defer s.close()
	status, content, err := s.do(fmt.Sprintf("GETSCRIPT %s", quoteSieve(name)))
	if err != nil {
		return "", err
	}
	if status != "OK" {
		return "", fmt.Errorf("sieve get: %s", status)
	}
	if len(content) == 0 {
		return "", nil
	}
	return content[0], nil
}

// SievePutScript creates or replaces a script and optionally activates it.
func (c *Client) SievePutScript(email, token, name, content string, activate bool) error {
	s, err := c.openSieve(email, token)
	if err != nil {
		return err
	}
	defer s.close()
	if err := s.writeLine(fmt.Sprintf("PUTSCRIPT %s {%d}\r\n%s", quoteSieve(name), len(content), content)); err != nil {
		return fmt.Errorf("sieve put: %w", err)
	}
	status, _, err := s.readResponse()
	if err != nil {
		return err
	}
	if status != "OK" {
		return fmt.Errorf("sieve put: %s", status)
	}
	if activate {
		return c.sieveSetActiveOn(s, name)
	}
	return nil
}

// SieveDeleteScript removes a script.
func (c *Client) SieveDeleteScript(email, token, name string) error {
	s, err := c.openSieve(email, token)
	if err != nil {
		return err
	}
	defer s.close()
	status, _, err := s.do(fmt.Sprintf("DELETESCRIPT %s", quoteSieve(name)))
	if err != nil {
		return err
	}
	if status != "OK" {
		return fmt.Errorf("sieve delete: %s", status)
	}
	return nil
}

// SieveSetActive activates (or, with an empty name, deactivates) a script.
func (c *Client) SieveSetActive(email, token, name string) error {
	s, err := c.openSieve(email, token)
	if err != nil {
		return err
	}
	defer s.close()
	return c.sieveSetActiveOn(s, name)
}

func (c *Client) sieveSetActiveOn(s *sieve, name string) error {
	status, _, err := s.do(fmt.Sprintf("SETACTIVE %s", quoteSieve(name)))
	if err != nil {
		return err
	}
	if status != "OK" {
		return fmt.Errorf("sieve setactive: %s", status)
	}
	return nil
}

// quoteSieve wraps a script name in a quoted string, escaping as needed.
func quoteSieve(name string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name)
	return `"` + escaped + `"`
}
