package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/haukened/gone/v3/internal/domain"
)

// v2Header is the parsed, bounds-checked 21-byte v2 blob header
// (docs/protocol.md §4.2).
type v2Header struct {
	iterations uint32
	salt       []byte
	raw        []byte
}

// SealV2 encrypts plaintext under protocol v2: the link key and a passphrase
// are combined so that neither alone can decrypt. A fresh random salt and
// nonce are drawn from crypto/rand and writers always use V2Iterations.
//
// Parameters:
//   - linkKey: KeySize-byte link key (the fragment key).
//   - passphrase: user passphrase; normalized to NFC, must be non-empty and
//     at most V2MaxPassphraseBytes UTF-8 bytes.
//   - plaintext: bytes to encrypt (raw or a GONE2 envelope).
//
// Returns the nonce and blob (hdr || ciphertext || tag), or ErrInvalidKey,
// ErrInvalidPassphrase, or an RNG/KDF error.
func SealV2(linkKey []byte, passphrase string, plaintext []byte) (nonce, blob []byte, err error) {
	return sealV2From(rand.Reader, linkKey, passphrase, plaintext)
}

// sealV2From is SealV2 with an injectable randomness source. The salt is
// read first, then the nonce, so vectors can be reproduced exactly.
//
// Parameters:
//   - r: randomness source for the salt and nonce.
//   - linkKey: KeySize-byte link key.
//   - passphrase: user passphrase.
//   - plaintext: bytes to encrypt.
//
// Returns the nonce and blob, or ErrInvalidKey, ErrInvalidPassphrase, or
// the read/KDF error.
func sealV2From(r io.Reader, linkKey []byte, passphrase string, plaintext []byte) (nonce, blob []byte, err error) {
	if len(linkKey) != domain.KeySize {
		return nil, nil, ErrInvalidKey
	}
	p, err := normalizePassphrase(passphrase)
	if err != nil {
		return nil, nil, err
	}
	defer clear(p)
	salt := make([]byte, domain.V2SaltSize)
	if _, err := io.ReadFull(r, salt); err != nil {
		return nil, nil, err
	}
	iv := make([]byte, domain.NonceSize)
	if _, err := io.ReadFull(r, iv); err != nil {
		return nil, nil, err
	}
	hdr := buildV2Header(domain.V2Iterations, salt)
	aead, err := v2AEAD(linkKey, p, salt, domain.V2Iterations)
	if err != nil {
		return nil, nil, err
	}
	return iv, aead.Seal(hdr, iv, plaintext, v2AAD(hdr)), nil
}

// OpenV2 authenticates and decrypts a v2 blob. Structural problems with the
// header return ErrMalformed (retrying cannot help); every authentication
// failure, including a wrong passphrase or link key, returns the single
// generic ErrDecrypt.
//
// Parameters:
//   - linkKey: KeySize-byte link key.
//   - passphrase: user passphrase; normalized to NFC before derivation.
//   - nonce: NonceSize-byte nonce from X-Gone-Nonce.
//   - blob: hdr || ciphertext || tag as stored by the server.
//
// Returns the plaintext, or ErrInvalidPassphrase, ErrMalformed, or
// ErrDecrypt.
func OpenV2(linkKey []byte, passphrase string, nonce, blob []byte) ([]byte, error) {
	if len(linkKey) != domain.KeySize || len(nonce) != domain.NonceSize {
		return nil, ErrDecrypt
	}
	hdr, err := parseV2Header(blob)
	if err != nil {
		return nil, err
	}
	p, err := normalizePassphrase(passphrase)
	if err != nil {
		return nil, err
	}
	defer clear(p)
	aead, err := v2AEAD(linkKey, p, hdr.salt, hdr.iterations)
	if err != nil {
		return nil, ErrDecrypt
	}
	pt, err := aead.Open(nil, nonce, blob[domain.V2HeaderSize:], v2AAD(hdr.raw))
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// normalizePassphrase applies the v2 passphrase rule: UTF-8 of the NFC
// form, non-empty, at most V2MaxPassphraseBytes, never trimmed.
//
// Parameters:
//   - passphrase: user input; must be valid UTF-8.
//
// Returns a fresh byte slice the caller may wipe, or ErrInvalidPassphrase.
func normalizePassphrase(passphrase string) ([]byte, error) {
	if !utf8.ValidString(passphrase) {
		return nil, ErrInvalidPassphrase
	}
	p := norm.NFC.Bytes([]byte(passphrase))
	if len(p) == 0 || len(p) > domain.V2MaxPassphraseBytes {
		return nil, ErrInvalidPassphrase
	}
	return p, nil
}

// derivePassphraseKey runs PBKDF2-HMAC-SHA-256 over the normalized
// passphrase.
//
// Parameters:
//   - p: normalized passphrase bytes.
//   - salt: V2SaltSize-byte salt from the header.
//   - iterations: PBKDF2 iteration count (already bounds-checked).
//
// Returns the 32-byte pw value or the KDF error.
func derivePassphraseKey(p, salt []byte, iterations uint32) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(p), salt, int(iterations), domain.KeySize)
}

// deriveV2Key combines the link key and pw with HKDF-SHA-256 into the
// AES-256 key: HKDF(IKM = linkKey || pw, salt = empty, info = V2HKDFInfo).
//
// Parameters:
//   - linkKey: KeySize-byte link key.
//   - pw: 32-byte PBKDF2 output.
//
// Returns the 32-byte AEAD key or the KDF error.
func deriveV2Key(linkKey, pw []byte) ([]byte, error) {
	ikm := make([]byte, 0, len(linkKey)+len(pw))
	ikm = append(append(ikm, linkKey...), pw...)
	defer clear(ikm)
	return hkdf.Key(sha256.New, ikm, nil, domain.V2HKDFInfo, domain.KeySize)
}

// v2AEAD derives the v2 key and builds its AES-256-GCM AEAD, wiping the
// intermediate key material before returning.
//
// Parameters:
//   - linkKey: KeySize-byte link key.
//   - p: normalized passphrase bytes.
//   - salt: header salt.
//   - iterations: header iteration count.
//
// Returns the AEAD or a KDF/cipher error.
func v2AEAD(linkKey, p, salt []byte, iterations uint32) (cipher.AEAD, error) {
	pw, err := derivePassphraseKey(p, salt, iterations)
	if err != nil {
		return nil, err
	}
	defer clear(pw)
	k, err := deriveV2Key(linkKey, pw)
	if err != nil {
		return nil, err
	}
	defer clear(k)
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// buildV2Header encodes kdf_id || iterations (uint32 BE) || salt.
//
// Parameters:
//   - iterations: PBKDF2 iteration count.
//   - salt: V2SaltSize-byte salt.
//
// Returns the V2HeaderSize-byte header.
func buildV2Header(iterations uint32, salt []byte) []byte {
	hdr := make([]byte, domain.V2HeaderSize)
	hdr[0] = domain.KDFPBKDF2SHA256
	binary.BigEndian.PutUint32(hdr[1:5], iterations)
	copy(hdr[5:], salt)
	return hdr
}

// parseV2Header validates the header at the start of a v2 blob: minimum
// length, a known KDF ID, and iterations within
// [V2MinIterations, V2MaxIterations].
//
// Parameters:
//   - blob: the full v2 blob.
//
// Returns the parsed header (sub-slices of blob) or ErrMalformed.
func parseV2Header(blob []byte) (v2Header, error) {
	if len(blob) < domain.V2HeaderSize+domain.TagSize {
		return v2Header{}, ErrMalformed
	}
	if blob[0] != domain.KDFPBKDF2SHA256 {
		return v2Header{}, ErrMalformed
	}
	iter := binary.BigEndian.Uint32(blob[1:5])
	if iter < domain.V2MinIterations || iter > domain.V2MaxIterations {
		return v2Header{}, ErrMalformed
	}
	return v2Header{
		iterations: iter,
		salt:       blob[5:domain.V2HeaderSize],
		raw:        blob[:domain.V2HeaderSize],
	}, nil
}

// v2AAD returns the v2 associated data: "gone:v2" || hdr.
//
// Parameters:
//   - hdr: the raw V2HeaderSize-byte header.
//
// Returns a fresh AAD slice.
func v2AAD(hdr []byte) []byte {
	aad := make([]byte, 0, len(domain.AADv2)+len(hdr))
	return append(append(aad, domain.AADv2...), hdr...)
}
