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
// (nginx.conf includes it conditionally).
func renderNginxAll(cfg NginxConfig) (map[string][]byte, error) {
	type file struct{ tmpl, dest string }
	if cfg.Engine != "mailezine" {
		files := []file{
			{"templates/nginx/nginx.conf.tmpl", "/etc/nginx/nginx.conf"},
			{"templates/nginx/proxy.conf.tmpl", "/etc/nginx/proxy.conf"},
		}
		// The dovecot login proxy only exists in the postdove engine mode;
		// the mailezine engine authenticates by itself.
		files = append(files,
			file{"templates/nginx/dovecot-proxy.conf.tmpl", "/etc/dovecot/proxy.conf"},
			file{"templates/nginx/login.lua.tmpl", "/etc/dovecot/login.lua"},
		)
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
	// mailezine mode: no nginx at all. The engine publishes the mail ports
	// itself (implicit TLS on 465/993/995) and caddy owns HTTP/ACME.
	files := []file{
		{"templates/nginx/Caddyfile.tmpl", "/etc/caddy/Caddyfile"},
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
