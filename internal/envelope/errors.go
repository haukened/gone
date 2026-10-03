package envelope

import "errors"

// Errors returned by this package. Callers should match with errors.Is.
var (
	// ErrDecrypt is the single generic error for every decryption failure
	// (bad key or nonce length, short ciphertext, failed authentication).
	ErrDecrypt = errors.New("envelope: decryption failed")
	// ErrMalformed indicates a structurally invalid v2 blob: too short, an
	// unknown KDF ID, or an iteration count outside the accepted range.
	// Unlike ErrDecrypt, retrying with another passphrase cannot help.
	ErrMalformed = errors.New("envelope: malformed ciphertext")
	// ErrInvalidPassphrase indicates a v2 passphrase that is empty, not valid
	// UTF-8, or longer than V2MaxPassphraseBytes after NFC normalization.
	ErrInvalidPassphrase = errors.New("envelope: invalid passphrase")
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
