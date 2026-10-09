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

// Protocol version 2 parameters (docs/protocol.md §4.2). v2 keeps the v1
// link key, nonce and AEAD, and adds a passphrase factor whose KDF
// parameters travel in an authenticated header prefixed to the ciphertext.
// The server never parses that header.
const (
	// ProtocolV2 is the link key + passphrase protocol version.
	ProtocolV2 uint8 = 2
	// AADv2 is the AAD prefix for v2; the full AAD is AADv2 || header.
	AADv2 = "gone:v2"
	// V2HKDFInfo is the HKDF info string that derives the v2 AEAD key.
	V2HKDFInfo = "gone:v2 aead key"
	// KDFPBKDF2SHA256 is the v2 KDF ID for PBKDF2-HMAC-SHA-256.
	KDFPBKDF2SHA256 byte = 0x01
	// V2SaltSize is the v2 KDF salt length in bytes.
	V2SaltSize = 16
	// V2HeaderSize is the v2 header length: kdf_id (1) || iterations (4) || salt.
	V2HeaderSize = 1 + 4 + V2SaltSize
	// V2Iterations is the PBKDF2 iteration count writers MUST use.
	V2Iterations = 600_000
	// V2MinIterations is the lowest iteration count readers accept.
	V2MinIterations = 600_000
	// V2MaxIterations is the highest iteration count readers accept; it
	// bounds the work a hostile sender can impose on a recipient.
	V2MaxIterations = 5_000_000
	// V2MaxPassphraseBytes caps the NFC-normalized UTF-8 passphrase length.
	V2MaxPassphraseBytes = 1024
)

// Supported reports whether v is a protocol version this build implements.
//
// Parameters:
//   - v: protocol version.
//
// Returns true for ProtocolV1 and ProtocolV2.
func Supported(v uint8) bool {
	return v == ProtocolV1 || v == ProtocolV2
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
// size. Every supported version uses a NonceSize-byte nonce.
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
	return validateNonce(nonce)
}

// Protocol version 3 parameters (docs/protocol.md §4.3). v3 encrypts a reply
// to a requester's ECDH P-256 public key. It is valid only on the request
// reply route, never for a secret created with POST /api/secret, so it is
// deliberately not in Supported.
const (
	// ProtocolV3 is the public-key reply protocol version.
	ProtocolV3 uint8 = 3
	// AADv3 is the AAD prefix for v3; the full AAD is AADv3 || header.
	AADv3 = "gone:v3"
	// V3HKDFInfo is the HKDF info prefix that derives the v3 AEAD key; the
	// full info is V3HKDFInfo || pubE || pubR.
	V3HKDFInfo = "gone:v3 aead key"
	// V3PublicKeySize is the length of an uncompressed P-256 point.
	V3PublicKeySize = 65
	// V3HeaderSize is the v3 header length: the ephemeral public key.
	V3HeaderSize = V3PublicKeySize
)

// ParseReplyVersion parses a protocol version like ParseVersion, but accepts
// only the reply protocol (ProtocolV3).
//
// Parameters:
//   - s: the version string from X-Gone-Version.
//
// Returns ProtocolV3, or ErrInvalidVersion.
func ParseReplyVersion(s string) (uint8, error) {
	n, ok := parseCanonicalDecimal(s, 3)
	if !ok || n != int(ProtocolV3) {
		return 0, ErrInvalidVersion
	}
	return ProtocolV3, nil
}

// ValidateReplyProtocol checks reply metadata: the version must be
// ProtocolV3 and the nonce strict base64url of NonceSize bytes.
//
// Parameters:
//   - v: protocol version.
//   - nonce: base64url-encoded nonce.
//
// Returns ErrInvalidVersion, ErrInvalidNonce, or nil.
func ValidateReplyProtocol(v uint8, nonce string) error {
	if v != ProtocolV3 {
		return ErrInvalidVersion
	}
	return validateNonce(nonce)
}

// validateNonce checks that nonce is strict base64url of NonceSize bytes.
//
// Parameters:
//   - nonce: base64url-encoded nonce.
//
// Returns ErrInvalidNonce or nil.
func validateNonce(nonce string) error {
	b, err := DecodeB64URL(nonce)
	if err != nil || len(b) != NonceSize {
		return ErrInvalidNonce
	}
	return nil
}
