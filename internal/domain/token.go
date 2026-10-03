// Package domain token.go contains the shared 256-bit bearer token primitive
// used by claim and manage tokens.
package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// bearerTokenBytes is the number of random bytes in a bearer token (256 bits).
const bearerTokenBytes = 32

// bearerTokenLen is the length of a base64url (unpadded) encoded bearer token.
const bearerTokenLen = 43

// newBearerToken generates a cryptographically random 256-bit token encoded as
// unpadded base64url.
//
// Returns the encoded token or an error if the system random source fails.
func newBearerToken() (string, error) {
	var b [bearerTokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// parseBearerToken validates that s is exactly 43 characters of canonical,
// unpadded base64url decoding to 32 bytes.
//
// Parameters:
//   - s: the candidate token string supplied by a client.
//   - invalid: the sentinel error returned when s is malformed.
//
// Returns s unchanged, or invalid if s is malformed.
func parseBearerToken(s string, invalid error) (string, error) {
	if len(s) != bearerTokenLen {
		return "", invalid
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != bearerTokenBytes {
		return "", invalid
	}
	return s, nil
}

// hashBearerToken returns the lowercase hex SHA-256 digest of a token string.
//
// Parameters:
//   - s: the encoded token.
//
// Returns the 64-character hex digest.
func hashBearerToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
