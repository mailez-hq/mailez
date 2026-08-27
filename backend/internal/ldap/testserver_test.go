package ldap

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimlambrt/gldap"
)

// testEntry is one directory record of the in-memory test server.
type testEntry struct {
	dn    string
	attrs map[string][]string
}

// testDirectory is a minimal but real LDAP server (gldap) for tests: simple
// bind by DN/uid + password, and search with attribute-filter matching
// (including & combinations and * wildcards).
type testDirectory struct {
	mu      sync.Mutex
	entries []*testEntry
}

func newTestServer(t *testing.T, entries []*testEntry) (host string, port int, td *testDirectory) {
	t.Helper()
	td = &testDirectory{entries: entries}
	s, err := gldap.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	mux, err := gldap.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	mux.Bind(td.handleBind)
	mux.Search(td.handleSearch)
	s.Router(mux)
	// Reserve a port and hand it to gldap.Run; if the OS gives the port to
	// someone else in the gap, Run returns immediately and we retry.
	for attempt := 0; attempt < 10 && port == 0; attempt++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		candidate := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		errCh := make(chan error, 1)
		go func() { errCh <- s.Run(fmt.Sprintf("127.0.0.1:%d", candidate)) }()
		select {
		case runErr := <-errCh:
			if runErr != nil {
				continue // port stolen; try another
			}
		case <-time.After(200 * time.Millisecond):
			port = candidate // Run is serving
		}
	}
	if port == 0 {
		t.Fatal("test server could not bind a port")
	}
	t.Cleanup(func() { _ = s.Stop() })
	return "127.0.0.1", port, td
}

func (td *testDirectory) handleBind(w *gldap.ResponseWriter, r *gldap.Request) {
	resp := r.NewBindResponse(gldap.WithResponseCode(gldap.ResultInvalidCredentials))
	defer func() { _ = w.Write(resp) }()
	m, err := r.GetSimpleBindMessage()
	if err != nil {
		return
	}
	td.mu.Lock()
	defer td.mu.Unlock()
	for _, e := range td.entries {
		if strings.EqualFold(e.dn, string(m.UserName)) || e.attrs["uid"][0] == string(m.UserName) {
			if e.attrs["password"][0] == string(m.Password) {
				resp.SetResultCode(gldap.ResultSuccess)
			}
			return
		}
	}
}

func (td *testDirectory) handleSearch(w *gldap.ResponseWriter, r *gldap.Request) {
	resp := r.NewSearchDoneResponse(gldap.WithResponseCode(gldap.ResultSuccess))
	defer func() { _ = w.Write(resp) }()
	m, err := r.GetSearchMessage()
	if err != nil {
		resp.SetResultCode(gldap.ResultOperationsError)
		return
	}
	td.mu.Lock()
	defer td.mu.Unlock()
	for _, e := range td.entries {
		if !strings.HasSuffix(e.dn, ","+string(m.BaseDN)) && !strings.EqualFold(e.dn, string(m.BaseDN)) {
			continue
		}
		if !matchEntryFilter(string(m.Filter), e) {
			continue
		}
		entry := r.NewSearchResponseEntry(e.dn)
		for name, vals := range e.attrs {
			entry.AddAttribute(name, vals)
		}
		_ = w.Write(entry)
	}
}

// matchEntryFilter evaluates a simplified LDAP filter: (attr=value),
// (attr=*), & conjunctions and ! negations.
func matchEntryFilter(filter string, e *testEntry) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	if strings.HasPrefix(filter, "(&") {
		all := true
		for _, sub := range splitFilterClauses(filter[2:]) {
			if !matchEntryFilter(sub, e) {
				all = false
			}
		}
		return all
	}
	if strings.HasPrefix(filter, "(|") {
		for _, sub := range splitFilterClauses(filter[2:]) {
			if matchEntryFilter(sub, e) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(filter, "(!") {
		subs := splitFilterClauses(filter[2:])
		return len(subs) == 1 && !matchEntryFilter(subs[0], e)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(filter, "("), ")")
	attr, val, found := strings.Cut(body, "=")
	if !found {
		return true
	}
	if val == "*" {
		for name := range e.attrs {
			if strings.EqualFold(name, attr) {
				return true
			}
		}
		return false
	}
	for name, vals := range e.attrs {
		if strings.EqualFold(name, attr) {
			for _, v := range vals {
				if strings.Contains(v, val) {
					return true
				}
			}
		}
	}
	return false
}

// splitFilterClauses splits a parenthesised filter list at top-level
// parentheses, e.g. "(&(a=1)(b=2))" -> ["(a=1)", "(b=2)"].
func splitFilterClauses(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(':
			if depth == 0 {
				start = i
			}
			depth++
		case ')':
			depth--
			if depth == 0 {
				out = append(out, s[start:i+1])
			}
		}
	}
	return out
}
