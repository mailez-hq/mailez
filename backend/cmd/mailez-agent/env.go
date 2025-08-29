package main

import (
	"os"
	"strings"
)

// envTrue reads a boolean-ish env var with a default.
func envTrue(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	}
	return def
}
