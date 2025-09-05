package macro

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"time"
)

// Server is the OLEFY/1.0-compatible TCP scanner service.
type Server struct {
	// MinLength is the minimum attachment size to scan.
	MinLength int
}

// Serve listens on addr (e.g. ":11343") and handles OLEFY/1.0 requests until
// ctx is done. PING is answered with PONG; Method: oletools payloads are
// scanned and answered with the olevba JSON array followed by the protocol
// stop word "\t\n\n\t".
func (s *Server) Serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(nowPlus(120))
	data, err := io.ReadAll(io.LimitReader(conn, 64<<20))
	if err != nil {
		return
	}

	// Header block ends at the first blank line; cap the header scan at 2000
	// bytes, mirroring the OLEFY scanner.
	probe := data
	if len(probe) > 2000 {
		probe = probe[:2000]
	}
	idx := bytes.Index(probe, []byte("\n\n"))
	headers := probe
	payload := data
	if idx >= 0 {
		headers = probe[:idx]
		payload = data[idx+2:]
	}

	head := string(headers)
	if strings.HasPrefix(head, "PING") {
		conn.Write([]byte("PONG"))
		return
	}
	if !strings.HasPrefix(head, "OLEFY/1.0") {
		writeOlefy(conn, ErrorProtocol())
		return
	}
	fields := parseOlefyHeaders(head)
	if fields["Method"] != "oletools" {
		writeOlefy(conn, ErrorMethod())
		return
	}
	if s.MinLength > 0 && len(payload) < s.MinLength {
		writeOlefy(conn, ErrorTooSmall())
		return
	}
	writeOlefy(conn, Scan(fields["Filename"], payload))
}

// parseOlefyHeaders parses "Key: Value" header lines (last value wins, like
// scanner dict overwrite).
func parseOlefyHeaders(head string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(head, "\n") {
		if line == "" || strings.HasPrefix(line, "OLEFY/1.0") {
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		if ok && k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// writeOlefy sends the response followed by the protocol's stop word.
func writeOlefy(w io.Writer, resp []byte) {
	w.Write(resp)
	w.Write([]byte("\t\n\n\t"))
}

// nowPlus is a tiny indirection so tests can keep imports light.
func nowPlus(seconds int) (t time.Time) {
	return time.Now().Add(time.Duration(seconds) * time.Second)
}
