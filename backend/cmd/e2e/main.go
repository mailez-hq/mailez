// Command e2e runs an end-to-end smoke test against a running mailez +
// the the mail stack mail stack: backend health, admin SSO login, domain/user provisioning,
// DKIM key generation, authenticated SMTP submission, IMAP delivery, and the
// DKIM-Signature / rspamd X-Spam headers on the delivered copy.
//
// Usage:
//
//	go run ./cmd/e2e [flags]
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	mailpk "mailez/backend/internal/mail"
)

type options struct {
	host          string
	apiPort       int
	smtpPort      int
	imapPort      int
	adminEmail    string
	adminPassword string
	domain        string
	user          string
	alias         string
	lenient       bool
}

func main() {
	opts := options{}
	flag.StringVar(&opts.host, "host", "127.0.0.1", "stack host")
	flag.IntVar(&opts.apiPort, "api-port", 8081, "backend API port")
	flag.IntVar(&opts.smtpPort, "smtp-port", 587, "SMTP submission port")
	flag.IntVar(&opts.imapPort, "imap-port", 143, "IMAP port")
	flag.StringVar(&opts.adminEmail, "admin-email", "admin@example.com", "global admin email")
	flag.StringVar(&opts.adminPassword, "admin-password", "MailezDemo2026!", "global admin password")
	flag.StringVar(&opts.domain, "domain", "e2e.example.com", "test domain")
	flag.StringVar(&opts.user, "user", "e2e", "test user localpart")
	flag.StringVar(&opts.alias, "alias", "", "alias localpart to test, e.g. team")
	flag.BoolVar(&opts.lenient, "lenient", false, "report DKIM/spam header issues without failing")
	flag.Parse()

	var checks int
	var passed int
	check := func(name string, ok bool, detail string) {
		checks++
		if ok {
			passed++
		}
		mark := "PASS"
		if !ok {
			mark = "FAIL"
		}
		if detail != "" {
			fmt.Printf("[%s] %s - %s\n", mark, name, detail)
		} else {
			fmt.Printf("[%s] %s\n", mark, name)
		}
	}
	fail := func(name string, err error) {
		check(name, false, err.Error())
	}

	base := fmt.Sprintf("http://%s:%d/api/v1", opts.host, opts.apiPort)
	api := newAPIClient(base)
	userEmail := opts.user + "@" + opts.domain
	userPassword := "E2e-Passw0rd!"

	// 1. Backend health.
	if code, err := api.json("GET", "/health", nil, nil); err != nil || code != 200 {
		fail("backend health", fmt.Errorf("status %d: %w", code, err))
		os.Exit(1)
	} else {
		check("backend health", true, fmt.Sprintf("status %d", code))
	}

	// 2. Admin SSO login (cookie jar keeps the session).
	if code, err := api.json("POST", "/sso/login",
		map[string]string{"email": opts.adminEmail, "pw": opts.adminPassword}, nil); err != nil || code != 200 {
		fail("admin login", fmt.Errorf("status %d: %w", code, err))
		os.Exit(1)
	} else {
		check("admin login", true, opts.adminEmail)
	}

	// 3. Provision test domain + user.
	{
		var domains []struct {
			Name string
		}
		if code, err := api.json("GET", "/domains", nil, &domains); err != nil || code != 200 {
			fail("list domains", fmt.Errorf("status %d: %w", code, err))
		} else if !containsName(domains, opts.domain) {
			code, err := api.json("POST", "/domains",
				map[string]any{"name": opts.domain, "max_users": -1, "max_aliases": -1}, nil)
			check("create test domain", err == nil && (code == 200 || code == 201), fmt.Sprintf("status %d", code))
		} else {
			check("create test domain", true, "already exists")
		}
	}
	{
		var users []struct {
			Email string
		}
		if code, err := api.json("GET", "/users", nil, &users); err != nil || code != 200 {
			fail("list users", fmt.Errorf("status %d: %w", code, err))
		} else if !containsEmail(users, userEmail) {
			code, err := api.json("POST", "/users",
				map[string]any{"email": userEmail, "password": userPassword, "enabled": true}, nil)
			check("create test user", err == nil && (code == 200 || code == 201), fmt.Sprintf("status %d", code))
		} else {
			check("create test user", true, "already exists")
		}
	}

	// 4. DKIM key generation.
	{
		var dkim struct {
			Record    string `json:"record"`
			PublicKey string `json:"public_key"`
			Enabled   bool   `json:"enabled"`
		}
		code, err := api.json("POST", "/domains/"+opts.domain+"/dkim", nil, &dkim)
		if err != nil || code != 200 {
			fail("dkim generate", fmt.Errorf("status %d: %w", code, err))
		} else {
			fmt.Printf("    DKIM record: %s TXT \"%s\"\n", dkim.Record, dkim.PublicKey)
			check("dkim generate", dkim.Enabled, "")
		}
	}

	// 5. SMTP submission (STARTTLS first, plain fallback, then port 25).
	subject := fmt.Sprintf("mailez-e2e-%d", time.Now().Unix())
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nE2E body",
		userEmail, userEmail, subject)
	sent := false
	for _, port := range []int{opts.smtpPort, 25} {
		if err := smtpSend(opts.host, port, userEmail, userPassword, userEmail, body); err == nil {
			fmt.Printf("    submitted via %s:%d\n", opts.host, port)
			sent = true
			break
		}
	}
	check("smtp send", sent, "")
	if !sent {
		os.Exit(1)
	}

	// 6. IMAP delivery (poll up to 60s).
	var raw []byte
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if msg, err := imapFetch(opts.host, opts.imapPort, userEmail, userPassword, subject); err == nil {
			raw = msg
			break
		}
		time.Sleep(5 * time.Second)
	}
	check("imap delivery", len(raw) > 0, "")
	if len(raw) == 0 {
		os.Exit(1)
	}

	// 7. Header checks on the delivered copy.
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	hasDKIM := err == nil && parsed.Header.Get("DKIM-Signature") != ""
	var spam []string
	if err == nil {
		for k := range parsed.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-spam") {
				spam = append(spam, k)
			}
		}
	}
	check("dkim signature header", hasDKIM, orDefault(hasDKIM, "present", "missing (rspamd signing not active?)"))
	check("rspamd spam headers", len(spam) > 0,
		orDefault(len(spam) > 0, "found: "+strings.Join(spam, ", "), "missing (rspamd not filtering?)"))
	if !opts.lenient && (!hasDKIM || len(spam) == 0) {
		fmt.Println("    (rerun with -lenient to report these without failing)")
		os.Exit(1)
	}

	// 8. Optional alias delivery.
	if opts.alias != "" {
		aliasEmail := opts.alias + "@" + opts.domain
		var aliases []struct {
			Email string
		}
		code, err := api.json("GET", "/aliases", nil, &aliases)
		if err != nil || code != 200 {
			fail("list aliases", fmt.Errorf("status %d: %w", code, err))
		} else if !containsEmail(aliases, aliasEmail) {
			api.json("POST", "/aliases", map[string]string{"email": aliasEmail, "destination": userEmail}, nil)
		}
		subject2 := fmt.Sprintf("mailez-e2e-alias-%d", time.Now().Unix())
		body2 := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nE2E alias body",
			userEmail, aliasEmail, subject2)
		ok := false
		for _, port := range []int{opts.smtpPort, 25} {
			if err := smtpSend(opts.host, port, userEmail, userPassword, aliasEmail, body2); err == nil {
				ok = true
				break
			}
		}
		if !ok {
			check("alias delivery", false, "smtp send failed")
			os.Exit(1)
		}
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if msg, err := imapFetch(opts.host, opts.imapPort, userEmail, userPassword, subject2); err == nil && len(msg) > 0 {
				ok = true
				break
			}
			time.Sleep(5 * time.Second)
		}
		check("alias delivery", ok, "")
	}

	fmt.Printf("\n%d/%d checks passed\n", passed, checks)
	if passed != checks {
		os.Exit(1)
	}
}

// apiClient talks to the management API with cookie-based sessions.
type apiClient struct {
	base string
	hc   *http.Client
}

func newAPIClient(base string) *apiClient {
	jar, _ := cookiejar.New(nil)
	return &apiClient{
		base: base,
		hc:   &http.Client{Jar: jar, Timeout: 15 * time.Second},
	}
}

func (a *apiClient) json(method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

// smtpSend delivers an authenticated message, trying STARTTLS then plaintext.
func smtpSend(host string, port int, user, pw, to, msg string) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Hello(host); err != nil {
		return err
	}
	// STARTTLS when offered; plaintext fallback for TLS_FLAVOR=notls.
	var auth smtp.Auth
	if err := c.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: true}); err == nil {
		auth = smtp.PlainAuth("", user, pw, host)
	} else {
		auth = mailpk.NewPlainAuth(user, pw)
	}
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(user); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// imapFetch returns the raw RFC822 message whose subject matches, if any.
func imapFetch(host string, port int, user, pw, subject string) ([]byte, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	c, err := client.Dial(addr)
	if err != nil {
		return nil, err
	}
	defer c.Logout()
	_ = c.StartTLS(&tls.Config{InsecureSkipVerify: true}) // plaintext fallback
	if err := c.Login(user, pw); err != nil {
		return nil, err
	}
	if _, err := c.Select("INBOX", true); err != nil {
		return nil, err
	}

	crit := imap.NewSearchCriteria()
	crit.Header = textproto.MIMEHeader{"Subject": []string{subject}}
	ids, err := c.Search(crit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("message %q not found", subject)
	}

	seq := new(imap.SeqSet)
	seq.AddNum(ids[len(ids)-1])
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchBodyStructure, section.FetchItem()}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seq, items, messages) }()
	var msg *imap.Message
	for m := range messages {
		msg = m
	}
	if err := <-done; err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("message %q not found", subject)
	}
	r := msg.GetBody(section)
	if r == nil {
		return nil, fmt.Errorf("no body section for %q", subject)
	}
	return io.ReadAll(r)
}

func containsName(list []struct{ Name string }, name string) bool {
	for _, v := range list {
		if v.Name == name {
			return true
		}
	}
	return false
}

func containsEmail(list []struct{ Email string }, email string) bool {
	for _, v := range list {
		if v.Email == email {
			return true
		}
	}
	return false
}

func orDefault(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

