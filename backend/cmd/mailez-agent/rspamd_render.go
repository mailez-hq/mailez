//go:build mailez_ee

package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"

	"mailez/backend/internal/ee/agent"
)

//go:embed templates/rspamd/*.tmpl
var rspamdTemplates embed.FS

// renderRspamdAll renders every embedded rspamd template into
// /etc/rspamd/local.d/<name>.
func renderRspamdAll(cfg RspamdConfig) (map[string][]byte, error) {
	entries, err := fs.Glob(rspamdTemplates, "templates/rspamd/*.tmpl")
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		data, err := renderRspamdTemplate(e, cfg)
		if err != nil {
			return nil, err
		}
		name := path.Base(strings.TrimSuffix(e, ".tmpl"))
		out["/etc/rspamd/local.d/"+name] = data
	}
	return out, nil
}

func renderRspamdTemplate(name string, data any) ([]byte, error) {
	t, err := template.New(name).ParseFS(rspamdTemplates, name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	base := path.Base(name)
	if err := t.Lookup(base).Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeRenderedRspamd atomically writes the rendered configs.
func writeRenderedRspamd(files map[string][]byte) error {
	for dest, data := range files {
		if err := agent.AtomicWrite(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}
