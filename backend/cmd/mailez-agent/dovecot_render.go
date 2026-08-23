package main

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"mailez/backend/internal/agent"
)

//go:embed templates/dovecot/*.tmpl
var dovecotTemplates embed.FS

// renderDovecotAll renders dovecot.conf plus the sieve execution scripts into
// destination -> content.
func renderDovecotAll(cfg DovecotConfig) (map[string][]byte, error) {
	out := make(map[string][]byte)

	data, err := renderDovecotTemplate("templates/dovecot/dovecot.conf.tmpl", cfg)
	if err != nil {
		return nil, err
	}
	out["/etc/dovecot/dovecot.conf"] = data

	// auth.conf is static.
	if auth, err := os.ReadFile("/conf/auth.conf"); err == nil {
		out["/etc/dovecot/auth.conf"] = auth
	} else {
		return nil, fmt.Errorf("read /conf/auth.conf: %w", err)
	}

	// ham/spam scripts: /conf/<name>.script -> /conf/bin/<name>, executable.
	for _, name := range []string{"ham", "spam"} {
		src := "/conf/" + name + ".script"
		body, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src, err)
		}
		// These scripts contain only {{ MAIL_FILTER_ADDRESS }}; render inline.
		rendered := strings.ReplaceAll(string(body), "{{ MAIL_FILTER_ADDRESS }}", agent.Getenv("MAIL_FILTER_ADDRESS", "mail-filter"))
		out[filepath.Join("/conf/bin", name)] = []byte(rendered)
	}
	return out, nil
}

func renderDovecotTemplate(name string, data any) ([]byte, error) {
	t, err := template.New(name).ParseFS(dovecotTemplates, name)
	if err != nil {
		return nil, err
	}
	base := name[strings.LastIndex(name, "/")+1:]
	var buf bytes.Buffer
	if err := t.Lookup(base).Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
