package main

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"mailez/backend/internal/agent"
)

//go:embed templates/postfix/*.tmpl
var postfixTemplates embed.FS

// renderPostfixAll renders the postfix config files into a destination ->
// content map. sasl_passwd and logrotate.conf are only produced when the
// corresponding env is set (matching start.py).
func renderPostfixAll(cfg PostfixConfig) (map[string][]byte, error) {
	files := map[string]string{
		"/etc/postfix/main.cf":                   "templates/postfix/main.cf.tmpl",
		"/etc/postfix/master.cf":                 "templates/postfix/master.cf.tmpl",
		"/etc/postfix/outclean_header_filter.cf": "templates/postfix/outclean_header_filter.cf.tmpl",
		"/etc/mta-sts-daemon.yml":                "templates/postfix/mta-sts-daemon.yml.tmpl",
	}
	out := make(map[string][]byte, len(files))
	for dest, tmpl := range files {
		data, err := renderPostfixTemplate(tmpl, cfg)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", tmpl, err)
		}
		out[dest] = data
	}
	return out, nil
}

func renderPostfixTemplate(name string, data any) ([]byte, error) {
	t, err := template.New(name).ParseFS(postfixTemplates, name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	base := name[strings.LastIndex(name, "/")+1:]
	if err := t.Lookup(base).Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeRenderedPostfix atomically writes every rendered file.
func writeRenderedPostfix(files map[string][]byte) error {
	for dest, data := range files {
		if err := agent.AtomicWrite(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}
