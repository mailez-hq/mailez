//go:build !mailez_ee

// eas-e2e smoke-tests ActiveSync; this build does not include the
// EAS stack, so the package ships a stub to stay buildable.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "eas-e2e: EAS is not part of this build")
	os.Exit(1)
}
