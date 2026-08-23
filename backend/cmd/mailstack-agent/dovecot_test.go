package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mailez/backend/internal/agent"
)

func TestDovecotRender(t *testing.T) {
	t.Setenv("HOSTNAMES", "mail.example.com")
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("POSTMASTER", "postmaster")
	t.Setenv("SUBNET", "192.168.206.0/24")
	t.Setenv("FULL_TEXT_SEARCH", "en,fr")
	t.Setenv("COMPRESSION", "zstd")

	cfg, err := loadDovecotConfig()
	if err != nil {
		t.Fatalf("loadDovecotConfig: %v", err)
	}
	if !cfg.FTSEnabled || cfg.FTSLanguages != "en fr" {
		t.Fatalf("FTS parse wrong: %+v", cfg)
	}
	if !cfg.CompressionEnabled || cfg.Compression != "zstd" {
		t.Fatalf("compression parse wrong: %+v", cfg)
	}
	if !strings.Contains(cfg.MailPlugins, "zlib") || !strings.Contains(cfg.MailPlugins, "fts fts_flatcurve") {
		t.Fatalf("mail plugins wrong: %q", cfg.MailPlugins)
	}

	data, err := renderDovecotTemplate("templates/dovecot/dovecot.conf.tmpl", cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	conf := string(data)
	for _, want := range []string{
		"postmaster_address = postmaster@example.com",
		"hostname = mail.example.com",
		"mail_location = maildir:/mail/%u",
		"quota_clone_dict = proxy:/tmp/mailez.socket:quota",
		"mailbox Trash {", "special_use = \\Trash",
		"fts_languages = en fr", "zlib_save = zstd",
		"sieve_before = dict:proxy:/tmp/mailez.socket:sieve",
		"!include_try /overrides/dovecot.conf",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("dovecot.conf missing %q", want)
		}
	}
}

func TestDictProtocol(t *testing.T) {
	var gotPath, gotPost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			gotPost = string(b)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		gotPath = r.URL.Path
		if strings.Contains(r.URL.Path, "/missing/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"value":"ok"}`))
	}))
	defer srv.Close()

	handler := agent.NewDictHandler(map[string]string{
		"auth": srv.URL + "/internal/dovecot/{}",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sock := filepath.Join(t.TempDir(), "mailez.socket")
	go handler.Serve(ctx, sock)

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mailez socket not created")
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	fmt.Fprintf(conn, "H1\t2\t0\tuser@example.com\tauth\n")
	fmt.Fprintf(conn, "Lpriv/quota/user@example.com\tuser@example.com\n")
	line, _ := r.ReadString('\n')
	if line != "O{\"value\":\"ok\"}\n" {
		t.Fatalf("lookup reply wrong: %q (path=%s)", line, gotPath)
	}
	if gotPath != "/internal/dovecot/quota/user@example.com/user@example.com" {
		t.Fatalf("priv lookup path wrong: %s", gotPath)
	}

	fmt.Fprintf(conn, "Lshared/passdb/e2e@example.com\te2e@example.com\n")
	line, _ = r.ReadString('\n')
	if line != "O{\"value\":\"ok\"}\n" {
		t.Fatalf("passdb lookup reply wrong: %q", line)
	}
	if gotPath != "/internal/dovecot/passdb/e2e@example.com" {
		t.Fatalf("passdb lookup path wrong: %s", gotPath)
	}

	fmt.Fprintf(conn, "Lshared/userdb/e2e@example.com\te2e@example.com\n")
	line, _ = r.ReadString('\n')
	if line != "O{\"value\":\"ok\"}\n" {
		t.Fatalf("userdb lookup reply wrong: %q", line)
	}
	if gotPath != "/internal/dovecot/userdb/e2e@example.com" {
		t.Fatalf("userdb lookup path wrong: %s", gotPath)
	}

	fmt.Fprintf(conn, "Lpriv/sieve/name/default\te2e@example.com\n")
	line, _ = r.ReadString('\n')
	if line != "O{\"value\":\"ok\"}\n" {
		t.Fatalf("sieve name lookup reply wrong: %q", line)
	}
	if gotPath != "/internal/dovecot/sieve/name/default/e2e@example.com" {
		t.Fatalf("sieve name path wrong: %s", gotPath)
	}

	// Pigeonhole quotes the JSON-encoded script name in the data key; the
	// control plane route is sieve/data/default/<user>.
	fmt.Fprintf(conn, "Lpriv/sieve/data/\"default\"\te2e@example.com\n")
	line, _ = r.ReadString('\n')
	if line != "O{\"value\":\"ok\"}\n" {
		t.Fatalf("sieve data lookup reply wrong: %q", line)
	}
	if gotPath != "/internal/dovecot/sieve/data/default/e2e@example.com" {
		t.Fatalf("sieve data path wrong: %s", gotPath)
	}

	fmt.Fprintf(conn, "Lpriv/missing/user@example.com\tuser@example.com\n")
	line, _ = r.ReadString('\n')
	if line != "N\n" {
		t.Fatalf("missing lookup reply wrong: %q", line)
	}

	fmt.Fprintf(conn, "B1\tuser@example.com\n")
	fmt.Fprintf(conn, "S1\tpriv/used\t1000\n")
	fmt.Fprintf(conn, "C1\n")
	line, _ = r.ReadString('\n')
	if line != "O\t1\n" {
		t.Fatalf("commit reply wrong: %q", line)
	}
	if gotPost != "1000" {
		t.Fatalf("set payload wrong: %q", gotPost)
	}
}

func TestTabEscapeRoundTrip(t *testing.T) {
	in := []byte("a\tb\nc\x01d")
	esc := agent.TabEscape(in)
	if strings.Contains(string(esc), "\t") {
		t.Fatalf("tab not escaped: %q", esc)
	}
	if got := string(agent.TabUnescape(esc)); got != string(in) {
		t.Fatalf("roundtrip mismatch: %q != %q", got, in)
	}
}

