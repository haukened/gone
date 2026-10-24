package cli

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"
)

// Byte-order marks some editors and shells write at the start of a text
// file: Windows PowerShell 5.1's ">" writes UTF-16LE, and older Notepad
// writes a UTF-8 BOM.
var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// decodeText turns text that starts with a byte-order mark into plain
// UTF-8: a UTF-8 BOM is dropped and UTF-16 (little- or big-endian) is
// converted. Text without a BOM is returned as it is. When b is converted,
// b is cleared, since it can hold a secret.
//
// Parameters:
//   - b: the bytes as read.
//
// Returns the UTF-8 text and what was done, to follow "The message" in a
// notice ("" when nothing was), or a usage error for UTF-16 with an odd
// number of bytes.
func decodeText(b []byte) ([]byte, string, error) {
	switch {
	case bytes.HasPrefix(b, bomUTF8):
		return b[len(bomUTF8):], "had a UTF-8 byte-order mark, which was removed", nil
	case bytes.HasPrefix(b, bomUTF16LE):
		return utf16ToUTF8(b, false)
	case bytes.HasPrefix(b, bomUTF16BE):
		return utf16ToUTF8(b, true)
	}
	return b, "", nil
}

// utf16ToUTF8 converts BOM-prefixed UTF-16 to UTF-8 and clears the input
// and the intermediate code units.
//
// Parameters:
//   - b: the bytes, starting with a two-byte BOM.
//   - bigEndian: the byte order the BOM names.
//
// Returns the UTF-8 text and a description, or a usage error.
func utf16ToUTF8(b []byte, bigEndian bool) ([]byte, string, error) {
	body := b[2:]
	if len(body)%2 != 0 {
		clear(b)
		return nil, "", usagef("the input starts like UTF-16 but has an odd number of bytes; save it as UTF-8")
	}
	units := make([]uint16, len(body)/2)
	for i := range units {
		hi, lo := body[2*i+1], body[2*i]
		if bigEndian {
			hi, lo = lo, hi
		}
		units[i] = uint16(hi)<<8 | uint16(lo)
	}
	clear(b)
	runes := utf16.Decode(units)
	clear(units)
	// Sized exactly, so append never leaves a partial copy behind.
	n := 0
	for _, r := range runes {
		n += utf8.RuneLen(r)
	}
	out := make([]byte, 0, n)
	for _, r := range runes {
		out = utf8.AppendRune(out, r)
	}
	clear(runes)
	return out, "was converted from UTF-16 to UTF-8", nil
}
