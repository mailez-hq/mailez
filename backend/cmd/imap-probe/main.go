// imap-probe: raw-protocol IMAP diagnostic (stdlib only).
// Prints SELECT/STATUS unseen counters and per-message flags for a folder.
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
	br.ReadString('\n')
	m := &conn{c: c, br: br}
	m.cmd(`LOGIN %q %q`, user, pass)
	return m
}

func (m *conn) cmd(format string, args ...any) string {
	m.n++
	tag := "p" + strconv.Itoa(m.n)
	fmt.Fprintf(m.c, tag+" "+format+"\r\n", args...)
	var sb strings.Builder
	for {
		_ = m.c.SetDeadline(time.Now().Add(15 * time.Second))
		line, err := m.br.ReadString('\n')
		if err != nil {
			break
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
	user, pass, folder := "admin@example.com", "MailezDemo2026!", "INBOX"
	if len(os.Args) > 2 {
		user, pass = os.Args[1], os.Args[2]
	}
	if len(os.Args) > 3 {
		folder = os.Args[3]
	}
	m := dial(addr, user, pass)
	sel := m.cmd("SELECT %q", folder)
	total := 0
	for _, l := range strings.Split(sel, "\r\n") {
		lt := strings.TrimSpace(l)
		if strings.Contains(lt, "EXISTS") || strings.Contains(lt, "UNSEEN") {
			fmt.Println("[SELECT]", lt)
		}
		if strings.HasSuffix(lt, " EXISTS") {
			fmt.Sscanf(lt, "* %d", &total)
		}
	}
	st := m.cmd("STATUS %q (UNSEEN)", folder)
	fmt.Println("[STATUS]", strings.ReplaceAll(strings.TrimSpace(st), "\r\n", " | "))
	resp := m.cmd("FETCH 1:* (UID FLAGS)")
	unseenCount := 0
	for _, l := range strings.Split(resp, "\r\n") {
		if strings.Contains(l, "FLAGS") {
			seen := strings.Contains(strings.ToLower(l), "\\seen")
			if !seen {
				unseenCount++
				fmt.Println("[UNSEEN-MSG]", strings.TrimSpace(l))
			}
		}
	}
	fmt.Printf("[SUMMARY] exists=%d fetch-unseen=%d\n", total, unseenCount)
	m.cmd("LOGOUT")
}
