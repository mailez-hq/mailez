package models

import (
	"encoding/json"
	"strings"
)

// Config stores in-database configuration values.
type Config struct {
	Name  string `gorm:"primaryKey;size:255;not null" json:"name"`
	Value string `gorm:"size:255" json:"value"`
}

// JSONValue decodes the stored value as JSON, returning nil on empty.
func (c *Config) JSONValue() any {
	if c.Value == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(c.Value), &v); err != nil {
		return nil
	}
	return v
}

// splitCSV parses a comma-separated list into a slice.
func splitCSV(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// joinCSV joins a slice into a comma-separated string.
func joinCSV(items []string) string {
	return strings.Join(items, ",")
}
