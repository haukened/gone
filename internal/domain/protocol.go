// Package domain protocol.go holds the protocol constants and validators
// shared by the server and the client-side envelope package (docs/protocol.md).
package domain

// Protocol version 1 parameters (docs/protocol.md §4).
const (
	// ProtocolV1 is the AES-256-GCM, raw-key-in-fragment protocol version.
	ProtocolV1 uint8 = 1
	// KeySize is the v1 AES-256 key length in bytes.
	KeySize = 32
	// NonceSize is the v1 AES-GCM nonce length in bytes.
	NonceSize = 12
	// TagSize is the v1 AES-GCM authentication tag length in bytes.
	TagSize = 16
	// AADv1 is the additional authenticated data bound to every v1 ciphertext.
	AADv1 = "gone:v1"
)

// Supported reports whether v is a protocol version this build implements.
//
// Parameters:
//   - v: protocol version.
//
// Returns true only for ProtocolV1.
func Supported(v uint8) bool {
	return v == ProtocolV1
}

// ParseVersion parses a protocol version written as canonical ASCII decimal
// (digits only, no sign, no leading zeros, 1-255) and requires that it is
// supported.
//
// Parameters:
//   - s: the version string, e.g. from X-Gone-Version or a link fragment.
//
// Returns the version, or ErrInvalidVersion.
func ParseVersion(s string) (uint8, error) {
	n, ok := parseCanonicalDecimal(s, 3)
	if !ok || n > 255 || !Supported(uint8(n)) { // #nosec G115 -- n <= 255 checked first
		return 0, ErrInvalidVersion
	}
	return uint8(n), nil // #nosec G115 -- n <= 255 checked above
}

// parseCanonicalDecimal parses a positive ASCII decimal with no sign and no
// leading zeros.
//
// Parameters:
//   - s: digits to parse.
//   - maxDigits: maximum length, small enough that the result cannot overflow.
//
// Returns the value and true, or 0 and false when s is not canonical.
func parseCanonicalDecimal(s string, maxDigits int) (int, bool) {
	if len(s) == 0 || len(s) > maxDigits || s[0] == '0' {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// ValidateProtocol checks stored protocol metadata: the version must be
// supported and the nonce must be strict base64url of that version's nonce
// size.
//
// Parameters:
//   - v: protocol version.
//   - nonce: base64url-encoded nonce.
//
// Returns ErrInvalidVersion, ErrInvalidNonce, or nil.
func ValidateProtocol(v uint8, nonce string) error {
	if !Supported(v) {
		return ErrInvalidVersion
	}
	b, err := DecodeB64URL(nonce)
	if err != nil || len(b) != NonceSize {
		return ErrInvalidNonce
	}
	return nil
}
