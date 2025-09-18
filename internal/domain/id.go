// Package domain id.go contains functions to generate, parse, and validate IDs
package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// SecretID is the canonical identifier for a stored secret.
// It is a 128-bit random value encoded as 32 lowercase hex characters.
type SecretID string

// NewID generates a new cryptographically random 128-bit SecretID encoded
// as 32 lowercase hexadecimal characters.
func NewID() (SecretID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	dst := make([]byte, 32)
	hex.Encode(dst, b[:]) // hex.Encode always produces lowercase
	return SecretID(dst), nil
}

// ParseID validates s and returns it as a SecretID. It enforces:
// - non-empty
// - length == 32
// - only lowercase [0-9a-f]
// Returns ErrInvalidID on failure.
func ParseID(s string) (SecretID, error) {
	if !isValidID(s) {
		return "", ErrInvalidID
	}
	return SecretID(s), nil
}

// String returns the string form of the SecretID.
func (id SecretID) String() string { return string(id) }

// Valid reports whether the ID satisfies the same rules as ParseID.
func (id SecretID) Valid() bool { return isValidID(string(id)) }

// isValidID performs validation without allocating errors.
func isValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
