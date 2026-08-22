package mail

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// fakeSieveServer serves a canned ManageSieve conversation over TCP.
func fakeSieveServer(t *testing.T, handler func(cmd string) []string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		bw := bufio.NewWriter(conn)
		_, _ = bw.WriteString("OK \"Dovecot ready.\"\r\n")
		_ = bw.Flush()
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimSpace(line)
			if strings.HasPrefix(cmd, "PUTSCRIPT") || strings.HasPrefix(cmd, "SETACTIVE") {
				// consume nothing extra; PUTSCRIPT literal body is ignored here
				if strings.HasPrefix(cmd, "PUTSCRIPT") {
					_, _ = br.ReadString('\n')
				}
			}
			for _, r := range handler(cmd) {
				_, _ = bw.WriteString(r + "\r\n")
			}
			_ = bw.Flush()
			if strings.HasPrefix(cmd, "LOGOUT") {
				return
			}
		}
	}()
	return ln.Addr().String()
}

func TestSieveListScripts(t *testing.T) {
	addr := fakeSieveServer(t, func(cmd string) []string {
		switch {
		case cmd == "STARTTLS":
			return []string{`NO "TLS not available"`}
		case strings.HasPrefix(cmd, "AUTHENTICATE"):
			return []string{`OK "Logged in."`}
		case cmd == "LISTSCRIPTS":
			return []string{`"default" ACTIVE`, `"rules"`, `OK "Listscripts completed."`}
		}
		return []string{`NO "unexpected"`}
	})
	c := &Client{SieveAddr: addr}
	scripts, err := c.SieveListScripts("amy@example.com", "tok")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(scripts) != 2 || scripts[0].Name != "default" || !scripts[0].Active ||
		scripts[1].Name != "rules" || scripts[1].Active {
		t.Fatalf("scripts: %+v", scripts)
	}
}

func TestSieveGetScript(t *testing.T) {
	addr := fakeSieveServer(t, func(cmd string) []string {
		switch {
		case cmd == "STARTTLS":
			return []string{`NO "TLS not available"`}
		case strings.HasPrefix(cmd, "AUTHENTICATE"):
			return []string{`OK "Logged in."`}
		case cmd == `GETSCRIPT "default"`:
			return []string{`{11}`, "hello world", `OK "Getscript completed."`}
		}
		return []string{`NO "unexpected"`}
	})
	c := &Client{SieveAddr: addr}
	content, err := c.SieveGetScript("amy@example.com", "tok", "default")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if content != "hello world" {
		t.Fatalf("content: %q", content)
	}
}

func TestSievePutAndActivate(t *testing.T) {
	var sawPut, sawActive string
	addr := fakeSieveServer(t, func(cmd string) []string {
		switch {
		case cmd == "STARTTLS":
			return []string{`NO "TLS not available"`}
		case strings.HasPrefix(cmd, "AUTHENTICATE"):
			return []string{`OK "Logged in."`}
		case strings.HasPrefix(cmd, "PUTSCRIPT"):
			sawPut = cmd
			return []string{`OK "Stored."`}
		case strings.HasPrefix(cmd, "SETACTIVE"):
			sawActive = cmd
			return []string{`OK "Activated."`}
		}
		return []string{`NO "unexpected"`}
	})
	c := &Client{SieveAddr: addr}
	if err := c.SievePutScript("amy@example.com", "tok", "rules", "if true { keep; }", true); err != nil {
		t.Fatalf("put: %v", err)
	}
	if !strings.Contains(sawPut, `"rules"`) {
		t.Fatalf("put command: %q", sawPut)
	}
	if !strings.Contains(sawActive, `"rules"`) {
		t.Fatalf("activate command: %q", sawActive)
	}
}

func TestSieveDeleteAndDeactivate(t *testing.T) {
	var sawDelete string
	addr := fakeSieveServer(t, func(cmd string) []string {
		switch {
		case cmd == "STARTTLS":
			return []string{`NO "TLS not available"`}
		case strings.HasPrefix(cmd, "AUTHENTICATE"):
			return []string{`OK "Logged in."`}
		case strings.HasPrefix(cmd, "DELETESCRIPT"):
			sawDelete = cmd
			return []string{`OK "Deleted."`}
		}
		return []string{`NO "unexpected"`}
	})
	c := &Client{SieveAddr: addr}
	if err := c.SieveDeleteScript("amy@example.com", "tok", "rules"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(sawDelete, `"rules"`) {
		t.Fatalf("delete command: %q", sawDelete)
	}
}

func TestQuoteSieve(t *testing.T) {
	if got := quoteSieve("my rules"); got != `"my rules"` {
		t.Errorf("plain: %q", got)
	}
	if got := quoteSieve(`a"b\c`); got != `"a\"b\\c"` {
		t.Errorf("escaped: %q", got)
	}
}

func TestStatusOf(t *testing.T) {
	if statusOf("OK") != "OK" || statusOf("OK \"done\"") != "OK" {
		t.Error("OK detection")
	}
	if statusOf("NO \"nope\"") != "NO" || statusOf("BYE bye") != "BYE" {
		t.Error("NO/BYE detection")
	}
	if statusOf(`"IMPLEMENTATION" "Dovecot"`) != "" {
		t.Error("capability line must not look like a status")
	}
}
