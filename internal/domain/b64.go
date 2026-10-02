package domain

import (
	"encoding/base64"
	"strings"
)

var b64 = base64.RawURLEncoding.Strict()

// b64URLAlphabet is the RFC 4648 §5 alphabet.
const b64URLAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// EncodeB64URL encodes b as unpadded base64url (RFC 4648 §5).
//
// Parameters:
//   - b: bytes to encode.
//
// Returns the encoded string.
func EncodeB64URL(b []byte) string {
	return b64.EncodeToString(b)
}

// DecodeB64URL decodes strict, canonical, unpadded base64url. Unlike the
// standard library decoder it also rejects CR and LF, so every byte string
// has exactly one accepted encoding (docs/protocol.md §2.1).
//
// Parameters:
//   - s: encoded string.
//
// Returns the decoded bytes, or ErrInvalidB64.
func DecodeB64URL(s string) ([]byte, error) {
	if !isB64URLAlphabet(s) {
		return nil, ErrInvalidB64
	}
	b, err := b64.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidB64
	}
	return b, nil
}

// isB64URLAlphabet reports whether every byte of s is in the base64url
// alphabet.
//
// Parameters:
//   - s: string to check.
//
// Returns true when s contains only A-Z, a-z, 0-9, '-' and '_'.
func isB64URLAlphabet(s string) bool {
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(b64URLAlphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}
