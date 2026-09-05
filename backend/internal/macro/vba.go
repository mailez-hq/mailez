// Package macro scans MS Office documents for VBA macros. It speaks the
// OLEFY/1.0 TCP protocol and returns olevba-compatible JSON so rspamd's
// oletools plugin works unchanged.
package macro

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

// Decompress implements the MS-OVBA 2.4.1 CompressedContainer decompression
// (a faithful port of olevba.decompress_stream).
func Decompress(container []byte) ([]byte, error) {
	if len(container) == 0 || container[0] != 0x01 {
		return nil, fmt.Errorf("invalid VBA signature byte %02X", container[0])
	}
	var out []byte
	pos := 1
	for pos < len(container) {
		chunkStart := pos
		if pos+2 > len(container) {
			return nil, fmt.Errorf("truncated chunk header")
		}
		header := binary.LittleEndian.Uint16(container[pos : pos+2])
		chunkSize := int(header&0x0FFF) + 3
		signature := (header >> 12) & 0x07
		if signature != 0b011 {
			return nil, fmt.Errorf("invalid CompressedChunkSignature %d", signature)
		}
		compressed := (header>>15)&0x01 == 1
		if compressed && chunkSize > 4098 {
			return nil, fmt.Errorf("CompressedChunkSize %d > 4098", chunkSize)
		}
		if !compressed && chunkSize != 4098 {
			return nil, fmt.Errorf("CompressedChunkSize %d != 4098 for raw chunk", chunkSize)
		}
		end := chunkStart + chunkSize
		if end > len(container) {
			end = len(container)
		}
		pos = chunkStart + 2
		if !compressed {
			n := end - pos
			if n > 4096 {
				n = 4096
			}
			out = append(out, container[pos:pos+n]...)
			pos += n
			continue
		}
		decompressedChunkStart := len(out)
		for pos < end {
			flagByte := container[pos]
			pos++
			for bit := 0; bit < 8; bit++ {
				if pos >= end {
					break
				}
				if flagByte&(1<<bit) == 0 {
					// LiteralToken: copy one byte
					out = append(out, container[pos])
					pos++
				} else {
					// CopyToken: 2 bytes, length + offset into the chunk
					if pos+2 > len(container) {
						return nil, fmt.Errorf("truncated copy token")
					}
					token := binary.LittleEndian.Uint16(container[pos : pos+2])
					lengthMask, offsetMask, bitCount, _ := copytokenHelp(len(out), decompressedChunkStart)
					length := int(token&lengthMask) + 3
					temp1 := token & offsetMask
					temp2 := 16 - bitCount
					offset := int(temp1>>temp2) + 1
					copySource := len(out) - offset
					if copySource < 0 {
						return nil, fmt.Errorf("copy offset %d before start of stream", offset)
					}
					for i := 0; i < length; i++ {
						out = append(out, out[copySource+i])
					}
					pos += 2
				}
			}
		}
	}
	return out, nil
}

// copytokenHelp computes the bit masks for a CopyToken (MS-OVBA 2.4.1.3.19.1).
func copytokenHelp(decompressedCurrent, decompressedChunkStart int) (lengthMask, offsetMask uint16, bitCount int, maximumLength int) {
	difference := decompressedCurrent - decompressedChunkStart
	bitCount = int(math.Ceil(math.Log2(float64(difference))))
	if bitCount < 4 {
		bitCount = 4
	}
	lengthMask = 0xFFFF >> bitCount
	offsetMask = ^lengthMask
	maximumLength = int(0xFFFF>>bitCount) + 3
	return
}

// DecodeModule converts a raw VBA module stream to source text: it
// decompresses MS-OVBA containers and decodes UTF-16LE / ANSI text.
func DecodeModule(raw []byte) string {
	data := raw
	if len(data) >= 2 && data[0] == 0x01 {
		if dec, err := Decompress(data); err == nil && len(dec) > 0 {
			data = dec
		}
	}
	if isUTF16(data) {
		return decodeUTF16LE(data)
	}
	return sanitizeText(data)
}

// isUTF16 reports whether data looks like UTF-16LE text (interleaved NULs in
// a long prefix, or a UTF-16 BOM).
func isUTF16(data []byte) bool {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return true
	}
	n := len(data)
	if n > 512 {
		n = 512
	}
	nulls := 0
	for i := 1; i < n; i += 2 {
		if data[i] == 0 {
			nulls++
		}
	}
	return n >= 8 && nulls*2 >= n/2
}

func decodeUTF16LE(data []byte) string {
	u := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		u = append(u, uint16(data[i])|uint16(data[i+1])<<8)
	}
	return sanitizeText([]byte(string(utf16.Decode(u))))
}

// sanitizeText keeps printable characters and newlines, dropping the binary
// noise that frm/userform streams carry around the source.
func sanitizeText(data []byte) string {
	var b bytes.Buffer
	b.Grow(len(data))
	for _, c := range data {
		if c == '\r' || c == '\n' || c == '\t' || (c >= 0x20 && c < 0x7F) {
			b.WriteByte(c)
		}
	}
	return b.String()
}
