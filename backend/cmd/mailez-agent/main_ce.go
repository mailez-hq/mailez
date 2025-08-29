//go:build !mailez_ee

// The mailez-agent binary manages the legacy MTA/IMAP stack/nginx stack
// (migration channel and bench baseline) and ships only in the enterprise
// edition. This stub keeps the package buildable in the community tree.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "mailez-agent: requires the enterprise edition build (-tags mailez_ee)")
	os.Exit(1)
}
