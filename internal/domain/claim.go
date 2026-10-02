// Package domain claim.go contains functions to generate, parse, and hash claim tokens.
package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// claimTokenBytes is the number of random bytes in a claim token (256 bits).
const claimTokenBytes = 32

// claimTokenLen is the length of a base64url (unpadded) encoded claim token.
const claimTokenLen = 43

// ClaimToken is a bearer credential issued to the first client that retrieves
// a secret. It authorizes retries of the download and the final acknowledgement
// within the claim lease. It is a 256-bit random value encoded as unpadded
// base64url (43 characters). Only its SHA-256 hash is ever persisted.
type ClaimToken string

// NewClaimToken generates a new cryptographically random ClaimToken.
//
// Returns the token or an error if the system random source fails.
func NewClaimToken() (ClaimToken, error) {
	var b [claimTokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ClaimToken(base64.RawURLEncoding.EncodeToString(b[:])), nil
}

// ParseClaimToken validates s and returns it as a ClaimToken. It enforces an
// exact length of 43 characters that decode as unpadded base64url to 32 bytes.
//
// Parameters:
//   - s: the candidate token string supplied by a client.
//
// Returns the token, or ErrInvalidClaim if s is malformed.
func ParseClaimToken(s string) (ClaimToken, error) {
	if len(s) != claimTokenLen {
		return "", ErrInvalidClaim
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != claimTokenBytes {
		return "", ErrInvalidClaim
	}
	return ClaimToken(s), nil
}

// String returns the string form of the ClaimToken.
func (t ClaimToken) String() string { return string(t) }

// Hash returns the lowercase hex SHA-256 digest of the token. This is the only
// representation that may be persisted.
func (t ClaimToken) Hash() string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}
