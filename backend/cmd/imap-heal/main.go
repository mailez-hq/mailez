// Command imap-heal bisects an INBOX for messages whose FETCH fails
// (dangling blob references: "store: not found"), marks them \Deleted and
// EXPUNGEs them, letting the engine rebuild a consistent snapshot.
// Throwaway operational tool — not for production use.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type conn struct {
	c  net.Conn
	br *bufio.Reader
	n  int
}

func dial(addr, user, pass string) *conn {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("dial:", err)
		os.Exit(1)
	}
	br := bufio.NewReader(c)
	br.ReadString('\n') // greeting
	m := &conn{c: c, br: br}
	m.cmd("LOGIN %q %q", user, pass)
	return m
}

func (m *conn) cmd(format string, args ...any) string {
	m.n++
	tag := "x" + strconv.Itoa(m.n)
	fmt.Fprintf(m.c, tag+" "+format+"\r\n", args...)
	var sb strings.Builder
	for {
		_ = m.c.SetDeadline(time.Now().Add(20 * time.Second))
		line, err := m.br.ReadString('\n')
		if err != nil {
			fmt.Println("read err:", err)
			os.Exit(1)
		}
		sb.WriteString(line)
		if strings.HasPrefix(line, tag+" ") {
			break
		}
	}
	return sb.String()
}

func main() {
	addr := "127.0.0.1:143"
	user, pass := "admin@example.com", "MailezDemo2026!"
	if len(os.Args) > 2 {
		user, pass = os.Args[1], os.Args[2]
	}
	m := dial(addr, user, pass)
	lst := m.cmd(`LIST "" "*"`)
	var folders []string
	for _, l := range strings.Split(lst, "\r\n") {
		if i := strings.LastIndex(l, ` "/" `); i >= 0 {
			name := strings.TrimSpace(strings.TrimPrefix(l[i+len(` "/" `):], `"`))
			name = strings.TrimSuffix(name, `"`)
			if name != "" {
				folders = append(folders, name)
			}
		}
	}
	fmt.Println("folders:", strings.Join(folders, ", "))
	totalBad := 0
	for _, folder := range folders {
		sel := m.cmd("SELECT %q", folder)
		total := 0
		for _, l := range strings.Split(sel, "\r\n") {
			l = strings.TrimSpace(l)
			if strings.HasSuffix(l, " EXISTS") {
				fmt.Sscanf(l, "* %d", &total)
			}
		}
		if total == 0 {
			fmt.Printf("%-28s empty\n", folder)
			continue
		}
		var bad []string
		for seq := 1; seq <= total; seq++ {
			// Probe with a header read: FLAGS live in the index and survive
			// blob loss, so only a body read exposes a dangling document.
			resp := m.cmd("FETCH %d (UID BODY.PEEK[HEADER])", seq)
			if strings.Contains(resp, " OK ") {
				continue
			}
			one := strings.ReplaceAll(strings.TrimSpace(resp), "\r\n", " | ")
			if len(one) > 100 {
				one = one[:100]
			}
			fmt.Printf("%-28s seq %-3d BAD: %s\n", folder, seq, one)
			bad = append(bad, strconv.Itoa(seq))
		}
		if len(bad) == 0 {
			fmt.Printf("%-28s %d msgs OK\n", folder, total)
			continue
		}
		totalBad += len(bad)
		set := strings.Join(bad, ",")
		if len(bad) == total {
			set = "1:*"
		}
		resp := m.cmd("STORE %s +FLAGS.SILENT (\\Deleted)", set)
		_ = resp
		resp = m.cmd("EXPUNGE")
		fmt.Printf("%-28s healed: expunged %d dangling of %d\n", folder, len(bad), total)
	}
	fmt.Println("total dangling expunged:", totalBad)
	m.cmd("LOGOUT")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\r\n")
	return lines[len(lines)-1]
}
