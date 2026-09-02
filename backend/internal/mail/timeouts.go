package mail

import "time"

// Transport timeouts for every protocol client in this package. The internal
// engine link was historically dialled and read with no deadlines at all: one
// engine that accepts a connection but never responds (crash mid-handshake,
// LB hiccup, stuck reader) parked one goroutine and one pooled API request
// forever, per request, until the process restarted. These bounds turn that
// failure mode into a fast, retryable error.
//
// The reference implementation is the external-account fetcher
// (internal/fetch/fetcher.go), which always had this shape.
const (
	// imapInternalDialTimeout bounds TCP connect to the local engine
	// (loopback / docker network: sub-second RTT, 10s is generous).
	imapInternalDialTimeout = 10 * time.Second
	// imapInternalCmdTimeout bounds each IMAP command round trip on internal
	// connections. Large FETCH windows (50 full messages + 300 envelopes) and
	// engine-side KV scans stay well inside it; IDLE must not use it (see
	// IdleWatch, which clears the timeout after dialling).
	imapInternalCmdTimeout = 2 * time.Minute

	// imapExternalDialTimeout / imapExternalCmdTimeout bound aggregated
	// external accounts on the public internet: higher latency, so a slower
	// dial and a slower per-command bound.
	imapExternalDialTimeout = 30 * time.Second
	imapExternalCmdTimeout  = 3 * time.Minute

	// SMTPDialTimeout bounds TCP connect (and TLS handshake, via
	// tls.DialWithDialer) to the submission target. Exported: the compose
	// outbox's direct MTA submission shares the same bounds.
	SMTPDialTimeout = 10 * time.Second
	// SMTPSessionTimeout is a hard ceiling on one whole submission — dial
	// through QUIT. 10 minutes carries the 50 MiB message cap over a ~90
	// KB/s uplink; a wedged peer converts to an error long before that.
	SMTPSessionTimeout = 10 * time.Minute

	// sieveCmdTimeout bounds one ManageSieve command round trip (greeting,
	// AUTHENTICATE, PUTSCRIPT literal, LISTSCRIPTS, ...). Applied per command
	// as a connection deadline; idle time between commands is not counted.
	sieveCmdTimeout = 60 * time.Second
)
