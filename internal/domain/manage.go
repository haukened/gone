// Package domain manage.go contains functions to generate, parse, and hash
// sender manage tokens.
package domain

// ManageToken is a bearer credential returned only to the sender when a secret
// is created. It authorizes checking whether the secret is still pending and
// revoking it. It is a 256-bit random value encoded as unpadded base64url (43
// characters). Only its SHA-256 hash is ever persisted.
type ManageToken string

// NewManageToken generates a new cryptographically random ManageToken.
//
// Returns the token or an error if the system random source fails.
func NewManageToken() (ManageToken, error) {
	s, err := newBearerToken()
	return ManageToken(s), err
}

// ParseManageToken validates s and returns it as a ManageToken. It enforces an
// exact length of 43 characters that decode as unpadded base64url to 32 bytes.
//
// Parameters:
//   - s: the candidate token string supplied by a client.
//
// Returns the token, or ErrInvalidManage if s is malformed.
func ParseManageToken(s string) (ManageToken, error) {
	v, err := parseBearerToken(s, ErrInvalidManage)
	return ManageToken(v), err
}

// String returns the string form of the ManageToken.
func (t ManageToken) String() string { return string(t) }

// Hash returns the lowercase hex SHA-256 digest of the token. This is the only
// representation that may be persisted.
func (t ManageToken) Hash() string { return hashBearerToken(string(t)) }
