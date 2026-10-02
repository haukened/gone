package envelope

import "errors"

// Errors returned by this package. Callers should match with errors.Is.
var (
	// ErrDecrypt is the single generic error for every v1 decryption failure
	// (bad key or nonce length, short ciphertext, failed authentication).
	ErrDecrypt = errors.New("envelope: decryption failed")
	// ErrInvalidKey indicates a key that is not KeySize bytes.
	ErrInvalidKey = errors.New("envelope: invalid key")
	// ErrInvalidFragment indicates a malformed link fragment or key payload.
	ErrInvalidFragment = errors.New("envelope: invalid fragment")
	// ErrInvalidLink indicates a link that is not an absolute http(s) URL of
	// the form <origin>/secret/<id>#<fragment>.
	ErrInvalidLink = errors.New("envelope: invalid link")
	// ErrInvalidEnvelope indicates a malformed GONE2 plaintext.
	ErrInvalidEnvelope = errors.New("envelope: invalid envelope")
	// ErrTooManyFiles indicates more than MaxFiles attachments.
	ErrTooManyFiles = errors.New("envelope: too many files")
	// ErrHeaderTooLarge indicates a GONE2 header over MaxHeaderBytes.
	ErrHeaderTooLarge = errors.New("envelope: header too large")
	// ErrInvalidMetadata indicates a file name or type that is not valid UTF-8.
	ErrInvalidMetadata = errors.New("envelope: invalid file metadata")
)
