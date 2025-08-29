//go:build !mailez_ee

// eas-e2e smoke-tests ActiveSync, an enterprise feature; the
// community build gets a stub so the package stays buildable.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "eas-e2e: requires the enterprise edition build (-tags mailez_ee)")
	os.Exit(1)
}
