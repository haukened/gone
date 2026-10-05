package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/haukened/gone/v3/internal/domain"
)

// NewKey returns a fresh random v1 key from crypto/rand.
//
// Returns a KeySize-byte key, or an error if the system RNG fails.
func NewKey() ([]byte, error) {
	return newKeyFrom(rand.Reader)
}

// newKeyFrom reads a v1 key from r. It exists so tests can inject a
// deterministic or failing reader.
//
// Parameters:
//   - r: randomness source.
//
// Returns a KeySize-byte key or the read error.
func newKeyFrom(r io.Reader) ([]byte, error) {
	k := make([]byte, domain.KeySize)
	if _, err := io.ReadFull(r, k); err != nil {
		return nil, err
	}
	return k, nil
}

// Seal encrypts plaintext under key with protocol v1 (AES-256-GCM, a fresh
// random 12-byte nonce, AAD "gone:v1").
//
// Parameters:
//   - key: KeySize-byte key.
//   - plaintext: bytes to encrypt.
//
// Returns the nonce and the ciphertext (with tag appended), or
// ErrInvalidKey, or an RNG error.
func Seal(key, plaintext []byte) (nonce, ciphertext []byte, err error) {
	return sealFrom(rand.Reader, key, plaintext)
}

// sealFrom is Seal with an injectable nonce source.
//
// Parameters:
//   - r: nonce randomness source.
//   - key: KeySize-byte key.
//   - plaintext: bytes to encrypt.
//
// Returns the nonce and ciphertext, or ErrInvalidKey or the read error.
func sealFrom(r io.Reader, key, plaintext []byte) (nonce, ciphertext []byte, err error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, nil, ErrInvalidKey
	}
	nonce = make([]byte, domain.NonceSize)
	if _, err := io.ReadFull(r, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, plaintext, []byte(domain.AADv1)), nil
}

// Open authenticates and decrypts a v1 ciphertext. Every failure, including
// wrong key or nonce lengths, returns ErrDecrypt so callers cannot build an
// oracle from the error.
//
// Parameters:
//   - key: KeySize-byte key.
//   - nonce: NonceSize-byte nonce.
//   - ciphertext: ciphertext with tag appended.
//
// Returns the plaintext or ErrDecrypt.
func Open(key, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil || len(nonce) != domain.NonceSize || len(ciphertext) < domain.TagSize {
		return nil, ErrDecrypt
	}
	pt, err := aead.Open(nil, nonce, ciphertext, []byte(domain.AADv1))
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// newAEAD builds the v1 AES-256-GCM AEAD for key.
//
// Parameters:
//   - key: must be exactly KeySize bytes (AES-128/192 are not accepted).
//
// Returns the AEAD or ErrInvalidKey.
func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != domain.KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
