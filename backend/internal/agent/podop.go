package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TabEscape escapes bytes using Dovecot's tabescape (strescape.c).
func TabEscape(b []byte) []byte {
	s := strings.NewReplacer(
		"\x01", "\x011",
		"\x00", "\x010",
		"\t", "\x01t",
		"\n", "\x01n",
		"\r", "\x01r",
	).Replace(string(b))
	return []byte(s)
}

// TabUnescape reverses Dovecot's tabescape.
func TabUnescape(b []byte) []byte {
	s := strings.NewReplacer(
		"\x01r", "\r",
		"\x01n", "\n",
		"\x01t", "\t",
		"\x010", "\x00",
		"\x011", "\x01",
	).Replace(string(b))
	return []byte(s)
}

// DictHandler serves the Dovecot dict protocol (podop replacement) over a
// unix socket, forwarding lookups and transactions to the control plane's
// internal API.
type DictHandler struct {
	// Tables maps dict names (auth/quota/sieve) to base URLs that contain a
	// "{}" placeholder for the key.
	Tables map[string]string
	Client *http.Client
}

func NewDictHandler(tables map[string]string) *DictHandler {
	return &DictHandler{
		Tables: tables,
		Client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Serve listens on socketPath and handles dict connections until ctx is done.
func (h *DictHandler) Serve(ctx context.Context, socketPath string) error {
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	// Dovecot connects as the mail user; the agent runs as root.
	_ = os.Chmod(socketPath, 0o666)
	defer ln.Close()

	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.handle(conn)
		}()
	}
}

func (h *DictHandler) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(b ...[]byte) { conn.Write(bytes.Join(b, nil)) }

	var tableURL, user, valueType string
	type setOp struct{ key, value string }
	tx := map[string]setOp{}
	txUser := ""

	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		line = bytes.TrimRight(line, "\r\n")
		fmt.Fprintf(os.Stderr, "podop: recv %q\n", line)
		if len(line) < 2 {
			continue
		}
		parts := bytes.Split(line[1:], []byte{'\t'})
		switch line[0] {
		case 'H': // H<major>\t<minor>\t<value_type>\t<user>\t<dict>
			if len(parts) >= 5 {
				tableURL = h.Tables[string(parts[4])]
				user = string(parts[3])
				valueType = string(parts[2])
			}
		case 'L': // L<key>\t[user]
			if len(parts) >= 1 && tableURL != "" {
				lu := user
				if len(parts) >= 2 {
					lu = string(parts[1])
				}
				if val, ok := h.lookup(tableURL, string(parts[0]), lu, valueType); ok {
					fmt.Fprintf(os.Stderr, "podop: lookup %s -> %s\n", parts[0], val)
					// Dovecot's dict protocol replies "O<value>\n" (no tab);
					// the vendored podop's "O\t<value>" is tolerated by some
					// dovecot builds but parsed as an empty value by others.
					write([]byte("O"), TabEscape(val), []byte("\n"))
				} else {
					fmt.Fprintf(os.Stderr, "podop: lookup %s -> not found\n", parts[0])
					write([]byte("N\n"))
				}
			}
		case 'I': // iterate: not implemented upstream beyond simple lists
			write([]byte("F\n"))
		case 'B': // B<transaction_id>\t[user]
			tx = map[string]setOp{}
			txUser = user
			if len(parts) >= 2 {
				txUser = string(parts[1])
			}
		case 'S': // S<transaction_id>\t<key>\t<value>
			if len(parts) >= 3 {
				tx[string(parts[0])] = setOp{key: string(parts[1]), value: string(parts[2])}
			}
		case 'C': // C<transaction_id>
			for _, op := range tx {
				h.set(tableURL, op.key, op.value, txUser)
			}
			tx = map[string]setOp{}
			write([]byte("O\t"), parts[0], []byte("\n"))
		}
	}
}

// dictKey converts a dict-protocol key into an API path segment, replicating
// podop's namespace handling: "priv/*" appends the user namespace, "shared/*"
// is stripped, and the remaining type prefix (passdb/userdb/quota/sieve) is
// kept in the path per the mailez control-plane contract.
func dictKey(key, user string) string {
	if i := strings.IndexByte(key, '/'); i >= 0 {
		typ, rest := key[:i], key[i+1:]
		switch typ {
		case "priv":
			return normalizeDictKey(rest) + "/" + user
		case "shared":
			return normalizeDictKey(rest)
		}
	}
	return normalizeDictKey(key)
}

// normalizeDictKey makes pigeonhole's sieve dict keys match the control-plane
// routes. The dict storage deliberately uses the JSON-encoded script name as
// the data id (e.g. sieve/data/"default" with literal quotes); the backend
// route is sieve/data/default/<user>, so the quotes are decoded here. Other
// keys pass through unchanged.
func normalizeDictKey(k string) string {
	const prefix = "sieve/data/"
	if !strings.HasPrefix(k, prefix) {
		return k
	}
	name := k[len(prefix):]
	if len(name) < 2 || name[0] != '"' || name[len(name)-1] != '"' {
		return k
	}
	if unquoted, err := strconv.Unquote(name); err == nil {
		return prefix + unquoted
	}
	return k
}

func escapePath(p string) string {
	e := url.PathEscape(p)
	return strings.ReplaceAll(e, "%2F", "/")
}

func (h *DictHandler) lookup(baseURL, key, user, valueType string) ([]byte, bool) {
	if baseURL == "" {
		return nil, false
	}
	u := strings.Replace(baseURL, "{}", escapePath(dictKey(key, user)), 1)
	resp, err := h.Client.Get(u)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	if valueType == "int" {
		return body, true
	}
	// Normalize: podop json.dumps()'d dict values; if backend already sent
	// JSON, pass through unchanged.
	return body, true
}

func (h *DictHandler) set(baseURL, key, value, user string) {
	if baseURL == "" {
		return
	}
	u := strings.Replace(baseURL, "{}", escapePath(dictKey(key, user)), 1)
	var payload any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		payload = value
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// PostfixHandler serves the postfix socketmap protocol (M4), mapping
// L<table>\t<key>\t<args> to the control plane internal API.
func PostfixSocketmapServe(ctx context.Context, socketPath string, urlFunc func(table, key string) string, client *http.Client) error {
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer ln.Close()

	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handleSocketmap(conn, urlFunc, client)
		}()
	}
}

func handleSocketmap(conn net.Conn, urlFunc func(table, key string) string, client *http.Client) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) < 2 {
			continue
		}
		parts := bytes.Split(line[1:], []byte{'\t'})
		if line[0] != 'L' || len(parts) < 2 {
			conn.Write([]byte("N\n"))
			continue
		}
		table := string(parts[0])
		key := string(parts[1])
		u := urlFunc(table, key)
		resp, err := client.Get(u)
		if err != nil || resp.StatusCode == http.StatusNotFound {
			conn.Write([]byte("N\n"))
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			conn.Write([]byte("N\n"))
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		conn.Write([]byte("O "))
		conn.Write(body)
		conn.Write([]byte("\n"))
	}
}
