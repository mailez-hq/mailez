package main

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"mailez/backend/internal/agent"
)

//go:embed templates/nginx/*.tmpl
var nginxTemplates embed.FS

// renderNginxAll renders every nginx/dovecot-proxy config into a
// destination -> content map. tls.conf is only produced when TLS is enabled
// (nginx.conf includes it conditionally). Both engine modes share the same
// HTTP/ACME nginx server; the dovecot login proxy only exists in postdove
// mode (the mailezine engine authenticates by itself).
func renderNginxAll(cfg NginxConfig) (map[string][]byte, error) {
	type file struct{ tmpl, dest string }
	files := []file{
		{"templates/nginx/nginx.conf.tmpl", "/etc/nginx/nginx.conf"},
		{"templates/nginx/proxy.conf.tmpl", "/etc/nginx/proxy.conf"},
	}
	if cfg.Engine != "mailezine" {
		files = append(files,
			file{"templates/nginx/dovecot-proxy.conf.tmpl", "/etc/dovecot/proxy.conf"},
			file{"templates/nginx/login.lua.tmpl", "/etc/dovecot/login.lua"},
		)
	}
	if cfg.TLS != nil {
		files = append(files, file{"templates/nginx/tls.conf.tmpl", "/etc/nginx/tls.conf"})
	}
	out := make(map[string][]byte, len(files))
	for _, f := range files {
		data, err := renderTemplate(f.tmpl, cfg)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", f.tmpl, err)
		}
		out[f.dest] = data
	}
	return out, nil
}

// renderNginxConfigs renders all configs and atomically writes them.
func renderNginxConfigs(cfg NginxConfig) error {
	files, err := renderNginxAll(cfg)
	if err != nil {
		return err
	}
	for dest, data := range files {
		if err := agent.AtomicWrite(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}

func renderTemplate(name string, data any) ([]byte, error) {
	t, err := template.New(name).ParseFS(nginxTemplates, name)
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
