package envelope

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Unpack decodes plaintext (docs/protocol.md §5.3). Plaintext without the
// magic is a text-only message. Message and file Data alias b; callers own
// b and should clear it when done. Names and types are re-sanitized.
//
// Parameters:
//   - b: decrypted plaintext.
//
// Returns the payload, or an error wrapping ErrInvalidEnvelope, or
// ErrTooManyFiles.
func Unpack(b []byte) (Payload, error) {
	if !bytes.HasPrefix(b, Magic[:]) {
		return Payload{Message: b}, nil
	}
	msgLen, hdrs, bodyStart, err := readHeader(b)
	if err != nil {
		return Payload{}, err
	}
	off := bodyStart + msgLen
	p := Payload{Message: b[bodyStart:off], Files: make([]File, len(hdrs))}
	for i, h := range hdrs {
		end := off + int(h.size) // #nosec G115 -- size <= len(b), checked by readHeader
		p.Files[i] = File{Name: h.name, Type: h.typ, Data: b[off:end]}
		off = end
	}
	return p, nil
}

// invalid wraps ErrInvalidEnvelope with a reason.
//
// Parameters:
//   - reason: short description.
//
// Returns the wrapped error.
func invalid(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidEnvelope, reason)
}

// readHeader validates the GONE2 prefix and header against b.
//
// Parameters:
//   - b: plaintext starting with Magic.
//
// Returns the message length, file headers, and body offset, or an error.
func readHeader(b []byte) (int, []fileHeader, int, error) {
	if len(b) < prefixLen {
		return 0, nil, 0, invalid("truncated")
	}
	hlen := binary.BigEndian.Uint32(b[len(Magic):prefixLen])
	avail := uint64(len(b) - prefixLen) // #nosec G115 -- len(b) >= prefixLen, checked above
	if hlen > MaxHeaderBytes || uint64(hlen) > avail {
		return 0, nil, 0, invalid("header length")
	}
	bodyStart := prefixLen + int(hlen)
	msg, hdrs, err := decodeHeader(b[prefixLen:bodyStart])
	if err != nil {
		return 0, nil, 0, err
	}
	if !sizesMatch(msg, hdrs, avail-uint64(hlen)) {
		return 0, nil, 0, invalid("size mismatch")
	}
	return int(msg), hdrs, bodyStart, nil // #nosec G115 -- msg <= len(b), checked by sizesMatch
}

// sizesMatch reports whether msg plus every file size equals avail exactly.
// The running total never exceeds avail, so it cannot overflow.
//
// Parameters:
//   - msg: message size.
//   - hdrs: file headers.
//   - avail: bytes after the header.
//
// Returns true on an exact match.
func sizesMatch(msg uint64, hdrs []fileHeader, avail uint64) bool {
	total := msg
	for _, h := range hdrs {
		if total > avail || h.size > avail-total {
			return false
		}
		total += h.size
	}
	return total == avail
}
