// mailstack-agent replaces the Python start.py scripts in the mailez
// mail-stack containers with a single static Go binary.
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
	case "version":
		fmt.Println("mailez mailstack-agent (unbound pilot)")
		return
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mailstack-agent:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mailstack-agent <component>")
	fmt.Fprintln(os.Stderr, "components: unbound")
}
