//go:build mailez_ee

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"mailez/backend/internal/ee/agent"
)

// MTA-STS policy daemon for postfix, a Go port of postfix-mta-sts-resolver
// (responder.py + resolver.py + internal_cache.py) with the same wire
// behavior: netstring-framed socketmap requests "<zone> <domain>" answered
// with "OK secure match=..." / "NOTFOUND ".

const (
	stsHardResponseLimit = 64 * 1024
	stsDefaultTimeout    = 4 * time.Second
	stsDefaultGrace      = 60 * time.Second
)

type stsConfig struct {
	Path            string        `yaml:"path"`
	ShutdownTimeout int           `yaml:"shutdown_timeout"`
	CacheGrace      time.Duration `yaml:"cache_grace"`
	Cache           struct {
		Type    string `yaml:"type"`
		Options struct {
			CacheSize int `yaml:"cache_size"`
		} `yaml:"options"`
	} `yaml:"cache"`
	DefaultZone stsZone            `yaml:"default_zone"`
	Zones       map[string]stsZone `yaml:"zones"`
}

type stsZone struct {
	StrictTesting bool `yaml:"strict_testing"`
	Timeout       int  `yaml:"timeout"`
	RequireSNI    bool `yaml:"require_sni"`
}

type stsPolicy struct {
	ID     string
	MaxAge int
	Mode   string
	MX     []string
}

type stsCacheEntry struct {
	ts     time.Time
	policy stsPolicy
}

type stsDaemon struct {
	cfg    stsConfig
	client *http.Client
	mu     sync.Mutex
	cache  map[string]stsCacheEntry
}

// serveMTASTS loads the rendered /etc/mta-sts-daemon.yml and serves postfix
// socketmap requests until ctx is done.
func serveMTASTS(ctx context.Context, configPath string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", configPath, err)
	}
	var cfg stsConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", configPath, err)
	}
	if cfg.Path == "" {
		cfg.Path = "/tmp/mta-sts.socket"
	}
	if cfg.DefaultZone.Timeout == 0 {
		cfg.DefaultZone.Timeout = 4
	}
	if cfg.CacheGrace == 0 {
		cfg.CacheGrace = stsDefaultGrace
	}
	for name := range cfg.Zones {
		if cfg.Zones[name].Timeout == 0 {
			z := cfg.Zones[name]
			z.Timeout = cfg.DefaultZone.Timeout
			cfg.Zones[name] = z
		}
	}

	d := &stsDaemon{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.DefaultZone.Timeout) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // no redirects, like the legacy daemon
			},
		},
		cache: make(map[string]stsCacheEntry, 10000),
	}

	_ = os.Remove(cfg.Path)
	ln, err := net.Listen("unix", cfg.Path)
	if err != nil {
		return err
	}
	// The agent runs as root but postfix connects as its own user.
	_ = os.Chmod(cfg.Path, 0o666)
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

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
			d.handle(conn)
		}()
	}
}

func (d *stsDaemon) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		req, err := agent.ReadNetstring(r)
		if err != nil {
			return
		}
		zone, domain, ok := strings.Cut(string(req), " ")
		if !ok {
			_ = agent.WriteNetstring(conn, []byte("NOTFOUND "))
			continue
		}
		_ = agent.WriteNetstring(conn, d.resolve(zone, filterSTSdomain(domain)))
	}
}

// resolve answers one "<zone> <domain>" lookup.
func (d *stsDaemon) resolve(zone, domain string) []byte {
	if domain == "" || strings.HasPrefix(domain, ".") || isIPLiteral(domain) {
		return []byte("NOTFOUND ")
	}
	z, ok := d.cfg.Zones[zone]
	if !ok {
		z = d.cfg.DefaultZone
	}

	now := time.Now()
	cached, haveCache := d.cacheGet(domain)
	if d.isStale(cached, haveCache, now) {
		if entry, ok := d.refresh(domain, cached, haveCache, now); ok {
			cached = entry
			haveCache = true
		}
	}

	if !haveCache || (cached.policy.MaxAge > 0 && cached.ts.Add(time.Duration(cached.policy.MaxAge)*time.Second).Before(now)) {
		return []byte("NOTFOUND ")
	}
	p := cached.policy
	if p.Mode == "none" || (p.Mode == "testing" && !z.StrictTesting) {
		return []byte("NOTFOUND ")
	}
	seen := map[string]bool{}
	var mxs []string
	for _, mx := range p.MX {
		mx = strings.TrimPrefix(mx, "*")
		if mx != "" && !seen[mx] {
			seen[mx] = true
			mxs = append(mxs, mx)
		}
	}
	resp := "OK secure match=" + strings.Join(mxs, ":")
	if z.RequireSNI {
		resp += " servername=hostname"
	}
	return []byte(resp)
}

func (d *stsDaemon) isStale(cached stsCacheEntry, haveCache bool, now time.Time) bool {
	if !haveCache {
		return true
	}
	if now.Sub(cached.ts) > d.cfg.CacheGrace {
		return true
	}
	return cached.policy.MaxAge > 0 && cached.ts.Add(time.Duration(cached.policy.MaxAge)*time.Second).Before(now)
}

// refresh performs a DNS + policy fetch for domain, updating the cache.
func (d *stsDaemon) refresh(domain string, cached stsCacheEntry, haveCache bool, now time.Time) (stsCacheEntry, bool) {
	lastID := ""
	if haveCache {
		lastID = cached.policy.ID
	}
	policy, status := d.fetch(domain, lastID)
	switch status {
	case "not_changed":
		cached.ts = now
		d.cacheSet(domain, cached)
		return cached, true
	case "valid":
		entry := stsCacheEntry{ts: now, policy: policy}
		d.cacheSet(domain, entry)
		return entry, true
	default:
		if haveCache && !(cached.policy.MaxAge > 0 && cached.ts.Add(time.Duration(cached.policy.MaxAge)*time.Second).Before(now)) {
			return cached, true
		}
		return stsCacheEntry{}, false
	}
}

// fetch resolves the MTA-STS TXT record and policy for domain. status is
// "valid", "not_changed" or "none".
func (d *stsDaemon) fetch(domain, lastID string) (stsPolicy, string) {
	txts, err := net.LookupTXT("_mta-sts." + domain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return stsPolicy{}, "none"
		}
		return stsPolicy{}, "none"
	}
	rec := ""
	for _, t := range txts {
		if strings.HasPrefix(t, "v=STSv1") {
			if rec != "" {
				return stsPolicy{}, "none" // more than one record
			}
			rec = t
		}
	}
	if rec == "" {
		return stsPolicy{}, "none"
	}
	fields := parseSTSFields(rec)
	if fields["v"] != "STSv1" || fields["id"] == "" {
		return stsPolicy{}, "none"
	}
	if fields["id"] == lastID {
		return stsPolicy{}, "not_changed"
	}

	policy, ok := d.fetchPolicy(domain)
	if !ok {
		return stsPolicy{}, "none"
	}
	policy.ID = fields["id"]
	return policy, "valid"
}

func (d *stsDaemon) fetchPolicy(domain string) (stsPolicy, bool) {
	url := "https://mta-sts." + domain + "/.well-known/mta-sts.txt"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return stsPolicy{}, false
	}
	req.Header.Set("User-Agent", "postfix-mta-sts-resolver")
	resp, err := d.client.Do(req)
	if err != nil {
		return stsPolicy{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return stsPolicy{}, false
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	if ct != "text/plain" {
		return stsPolicy{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, stsHardResponseLimit+1))
	if err != nil || len(body) > stsHardResponseLimit {
		return stsPolicy{}, false
	}

	p := parseSTSPolicy(string(body))
	if p.Mode != "none" && len(p.MX) == 0 {
		return stsPolicy{}, false
	}
	if p.MaxAge < 0 || p.MaxAge > 31557600 {
		return stsPolicy{}, false
	}
	return p, true
}

// parseSTSFields parses a semicolon-separated key=value record.
func parseSTSFields(rec string) map[string]string {
	out := map[string]string{}
	for _, field := range strings.Split(rec, ";") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		k, v, ok := strings.Cut(field, "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

// parseSTSPolicy parses the "key: value" policy file. Duplicate mx keys
// accumulate (the legacy parser appends to mx, overwrites other keys).
func parseSTSPolicy(text string) stsPolicy {
	p := stsPolicy{Mode: "none", MaxAge: -1}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "mx":
			p.MX = append(p.MX, value)
		case "max_age":
			if n, err := strconv.Atoi(value); err == nil {
				p.MaxAge = n
			}
		case "mode":
			p.Mode = value
		case "version":
			if value != "STSv1" {
				p.Mode = "invalid"
			}
		}
	}
	return p
}

func filterSTSdomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if i := strings.Index(domain, "]"); i >= 0 {
		domain = strings.TrimPrefix(domain[:i], "[")
	} else if i := strings.LastIndex(domain, ":"); i >= 0 {
		domain = domain[:i]
	}
	return strings.TrimSuffix(domain, ".")
}

func isIPLiteral(domain string) bool {
	return net.ParseIP(domain) != nil
}

func (d *stsDaemon) cacheGet(domain string) (stsCacheEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.cache[domain]
	return e, ok
}

func (d *stsDaemon) cacheSet(domain string, e stsCacheEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.cache) >= 10000 {
		d.cache = make(map[string]stsCacheEntry, 10000)
	}
	d.cache[domain] = e
}
