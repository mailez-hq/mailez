// mailstack-agent drives each mail-stack container in the mailez
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
	case "nginx":
		err = runNginx()
	case "dovecot":
		err = runDovecot()
	case "postfix":
		err = runPostfix()
	case "rspamd":
		err = runRspamd()
	case "oletools":
		err = runOletools()
	case "version":
		fmt.Println("mailez mailstack-agent (unbound + nginx + dovecot + postfix + rspamd + oletools)")
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
	fmt.Fprintln(os.Stderr, "components: unbound nginx dovecot postfix rspamd oletools")
}
