// mailez-agent drives every shared mail container (gateway / mail-filter /
// macro-scanner / resolver) with a single static Go binary.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "unbound":
		err = runUnbound()
	case "nginx":
		err = runNginx()
	case "rspamd":
		err = runRspamd()
	case "macro-scanner":
		err = runMacroScanner()
	case "version":
		fmt.Println("mailez-agent (unbound + nginx + rspamd + macro-scanner)")
		return
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mailez-agent:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mailez-agent <component>")
	fmt.Fprintln(os.Stderr, "components: unbound nginx rspamd macro-scanner")
}
