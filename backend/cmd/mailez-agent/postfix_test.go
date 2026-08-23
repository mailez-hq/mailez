package main

import (
	"bufio"
	"context"
	"fmt"
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

func TestPostfixRender(t *testing.T) {
	t.Setenv("MAILEZ_HOSTNAMES", "mail.example.com")
	t.Setenv("MAILEZ_DOMAIN", "example.com")
	t.Setenv("MAILEZ_SUBNET", "192.168.206.0/24")
	t.Setenv("MAILEZ_SUBNET6", "fdc4:f303:9324::254/64")
	t.Setenv("MAILEZ_RELAYNETS", "10.0.0.0/8,fd00::/8")
	t.Setenv("MAILEZ_RELAYHOST", "relay.example.net")
	t.Setenv("MAILEZ_RELAYUSER", "relayuser")
	t.Setenv("MAILEZ_DEFER_ON_TLS_ERROR", "true")
	t.Setenv("MAILEZ_REJECT_UNLISTED_RECIPIENT", "yes")

	cfg, err := loadPostfixConfig()
	if err != nil {
		t.Fatalf("loadPostfixConfig: %v", err)
	}
	if cfg.Hostname != "mail.example.com" {
		t.Errorf("hostname wrong: %s", cfg.Hostname)
	}
	if !strings.Contains(cfg.MyNetworks, "127.0.0.1/32") ||
		!strings.Contains(cfg.MyNetworks, "[fdc4:f303:9324::254]/64") ||
		!strings.Contains(cfg.MyNetworks, "[fd00::]/8") {
		t.Errorf("mynetworks wrong: %q", cfg.MyNetworks)
	}
	if cfg.AuthorizedXclient != "192.168.206.0/24,[fdc4:f303:9324::254]/64" {
		t.Errorf("authorized xclient wrong: %q", cfg.AuthorizedXclient)
	}

	files, err := renderPostfixAll(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	main := string(files["/etc/postfix/main.cf"])
	for _, want := range []string{
		"mydomain = example.com",
		"myhostname = mail.example.com",
		"relayhost = relay.example.net",
		"smtp_sasl_auth_enable = yes",
		"smtp_tls_security_level = dane",
		"smtp_tls_dane_insecure_mx_policy = dane",
		"virtual_transport = lmtp:inet:gateway:2525",
		"smtpd_milters = inet:mail-filter:11332",
		"smtpd_authorized_xclient_hosts=192.168.206.0/24,[fdc4:f303:9324::254]/64",
		"socketmap:unix:/tmp/mta-sts.socket:postfix",
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.cf missing %q", want)
		}
	}
	master := string(files["/etc/postfix/master.cf"])
	if !strings.Contains(master, "smtpd_reject_unlisted_recipient=yes") {
		t.Errorf("master.cf missing reject flag: %q", master)
	}
	sts := string(files["/etc/mta-sts-daemon.yml"])
	if !strings.Contains(sts, "strict_testing: true") {
		t.Errorf("mta-sts yml wrong: %q", sts)
	}
}

func TestPostfixRenderNotlsDefaults(t *testing.T) {
	t.Setenv("MAILEZ_HOSTNAMES", "mail.example.com")
	t.Setenv("MAILEZ_SUBNET", "192.168.206.0/24")
	t.Setenv("MAILEZ_DEFER_ON_TLS_ERROR", "false")
	cfg, err := loadPostfixConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	files, err := renderPostfixAll(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	main := string(files["/etc/postfix/main.cf"])
	if !strings.Contains(main, "smtp_tls_dane_insecure_mx_policy = may") {
		t.Errorf("dane policy wrong: %q", main)
	}
	master := string(files["/etc/postfix/master.cf"])
	if !strings.Contains(master, "smtpd_reject_unlisted_recipient=no") {
		t.Errorf("master.cf reject default wrong: %q", master)
	}
}

func TestPostfixSocketmapProtocol(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		switch {
		case strings.Contains(r.URL.Path, "/domain/"):
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(r.URL.Path, "/alias/"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`"team@example.com"`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`"ok"`))
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sock := filepath.Join(t.TempDir(), "mailez.socket")
	urlFor := func(table, key string) string {
		if suffix, ok := postfixTables[table]; ok {
			return srv.URL + "/stack/postfix/" + suffix + key
		}
		return ""
	}
	go agent.PostfixSocketmapServe(ctx, sock, urlFor, srv.Client())

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socketmap socket not created")
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	send := func(payload string) string {
		if _, err := fmt.Fprintf(conn, "%d:%s,", len(payload), payload); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := agent.ReadNetstring(r)
		if err != nil {
			t.Fatalf("read reply: %v", err)
		}
		return string(got)
	}

	if got := send("alias team@example.com"); got != "OK team@example.com" {
		t.Fatalf("alias reply wrong: %q (path=%s)", got, gotPath)
	}
	if gotPath != "/stack/postfix/alias/team@example.com" {
		t.Fatalf("alias path wrong: %s", gotPath)
	}
	if got := send("recipientmap SRS0+xxx=xx=example.com=alice@example.com"); got != "OK ok" {
		t.Fatalf("recipientmap reply wrong: %q (path=%s)", got, gotPath)
	}
	if got := send("domain missing.example"); got != "NOTFOUND " {
		t.Fatalf("missing reply wrong: %q", got)
	}
	if got := send("nosuchtable x"); got != "TEMP no such map" {
		t.Fatalf("unknown table reply wrong: %q", got)
	}
}

func TestParseSTSPolicy(t *testing.T) {
	text := "version: STSv1\nmode: enforce\nmax_age: 604800\nmx: mail.example.com\nmx: mx2.example.com\n"
	p := parseSTSPolicy(text)
	if p.Mode != "enforce" || p.MaxAge != 604800 || len(p.MX) != 2 || p.MX[1] != "mx2.example.com" {
		t.Fatalf("policy parse wrong: %+v", p)
	}
	fields := parseSTSFields("v=STSv1; id=20230801;")
	if fields["v"] != "STSv1" || fields["id"] != "20230801" {
		t.Fatalf("record parse wrong: %+v", fields)
	}
}
