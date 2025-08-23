package fetch

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// pop3Conn is a minimal POP3 (RFC 1939) client sufficient for fetching mail,
// avoiding an external dependency. It supports implicit TLS (port 995) when
// the Fetch.TLS flag is set.
type pop3Conn struct {
	conn net.Conn
	r    *bufio.Reader
}

func dialPOP3(host string, port int, useTLS, insecure bool) (*pop3Conn, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var conn net.Conn
	var err error
	if useTLS {
		conn, err = tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: insecure, ServerName: host})
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	c := &pop3Conn{conn: conn, r: bufio.NewReader(conn)}
	if _, err := c.readline(); err != nil { // greeting +OK
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (p *pop3Conn) readline() (string, error) {
	line, err := p.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// cmd sends a command and returns the single-line status response.
func (p *pop3Conn) cmd(line string) (string, error) {
	if _, err := fmt.Fprintf(p.conn, "%s\r\n", line); err != nil {
		return "", err
	}
	return p.readline()
}

// multiline returns the status line plus all data lines until the "." terminator.
func (p *pop3Conn) multiline(line string) (string, []string, error) {
	status, err := p.cmd(line)
	if err != nil {
		return "", nil, err
	}
	if !strings.HasPrefix(status, "+OK") {
		return status, nil, nil
	}
	var lines []string
	for {
		l, err := p.readline()
		if err != nil {
			return "", nil, err
		}
		if l == "." {
			break
		}
		lines = append(lines, l)
	}
	return status, lines, nil
}

func (p *pop3Conn) auth(user, pass string) error {
	status, err := p.cmd("USER " + user)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(status, "+OK") {
		return fmt.Errorf("pop3 USER rejected: %s", status)
	}
	status, err = p.cmd("PASS " + pass)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(status, "+OK") {
		return fmt.Errorf("pop3 PASS rejected: %s", status)
	}
	return nil
}

func (p *pop3Conn) stat() (int, error) {
	status, err := p.cmd("STAT")
	if err != nil {
		return 0, err
	}
	if !strings.HasPrefix(status, "+OK") {
		return 0, fmt.Errorf("pop3 STAT: %s", status)
	}
	parts := strings.Fields(status)
	if len(parts) < 2 {
		return 0, fmt.Errorf("pop3 STAT: malformed %q", status)
	}
	return strconv.Atoi(parts[1])
}

func (p *pop3Conn) uidl() ([]string, error) {
	_, lines, err := p.multiline("UIDL")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 2 {
			ids = append(ids, fields[1])
		}
	}
	return ids, nil
}

// retr downloads one message; lines are rejoined with CRLF.
func (p *pop3Conn) retr(n int) (string, error) {
	status, lines, err := p.multiline(fmt.Sprintf("RETR %d", n))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(status, "+OK") {
		return "", fmt.Errorf("pop3 RETR: %s", status)
	}
	return strings.Join(lines, "\r\n"), nil
}

func (p *pop3Conn) dele(n int) error {
	status, err := p.cmd(fmt.Sprintf("DELE %d", n))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(status, "+OK") {
		return fmt.Errorf("pop3 DELE: %s", status)
	}
	return nil
}

func (p *pop3Conn) quit() error {
	_, _ = p.cmd("QUIT")
	return p.conn.Close()
}
