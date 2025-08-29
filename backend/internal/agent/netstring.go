package agent

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// ReadNetstring reads a netstring-framed payload (len:bytes,) from r.
// Shared by the socketmap-style protocols the mail components speak.
func ReadNetstring(r *bufio.Reader) ([]byte, error) {
	var lenBuf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == ':' {
			break
		}
		if b < '0' || b > '9' {
			return nil, fmt.Errorf("socketmap: invalid netstring length byte %q", b)
		}
		lenBuf = append(lenBuf, b)
		if len(lenBuf) > 10 {
			return nil, fmt.Errorf("socketmap: netstring length too long")
		}
	}
	n, err := strconv.Atoi(string(lenBuf))
	if err != nil || n < 0 || n > 65535 {
		return nil, fmt.Errorf("socketmap: invalid netstring length %q", lenBuf)
	}
	payload := make([]byte, n+1) // payload + trailing comma
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	if payload[n] != ',' {
		return nil, fmt.Errorf("socketmap: netstring missing trailing comma")
	}
	return payload[:n], nil
}

// WriteNetstring writes a netstring-framed payload to w.
func WriteNetstring(w io.Writer, payload []byte) error {
	if _, err := fmt.Fprintf(w, "%d:", len(payload)); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err := w.Write([]byte{','})
	return err
}
