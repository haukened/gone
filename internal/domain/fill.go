// Package domain fill.go contains functions to generate, parse, and hash
// request fill tokens.
package domain

// FillToken is a bearer credential returned to the requester when a secret
// request is created and carried in the reply link's fragment. It authorizes
// checking that the request is open and sending its single reply. It is a
// 256-bit random value encoded as unpadded base64url (43 characters). Only its
// SHA-256 hash is ever persisted.
type FillToken string

// NewFillToken generates a new cryptographically random FillToken.
//
// Returns the token or an error if the system random source fails.
func NewFillToken() (FillToken, error) {
	s, err := newBearerToken()
	return FillToken(s), err
}

// ParseFillToken validates s and returns it as a FillToken. It enforces an
// exact length of 43 characters that decode as unpadded base64url to 32 bytes.
//
// Parameters:
//   - s: the candidate token string supplied by a client.
//
// Returns the token, or ErrInvalidFill if s is malformed.
func ParseFillToken(s string) (FillToken, error) {
	v, err := parseBearerToken(s, ErrInvalidFill)
	return FillToken(v), err
}

// String returns the string form of the FillToken.
func (t FillToken) String() string { return string(t) }

// Hash returns the lowercase hex SHA-256 digest of the token. This is the only
// representation that may be persisted.
func (t FillToken) Hash() string { return hashBearerToken(string(t)) }
