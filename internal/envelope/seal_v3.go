package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/haukened/gone/v3/internal/domain"
)

// NewRequestKey returns a fresh ECDH P-256 key pair for a secret request
// (docs/protocol.md §4.3). Its public key goes in the reply link; the private
// key never leaves the requester.
//
// Returns the private key, or an error if the system RNG fails.
func NewRequestKey() (*ecdh.PrivateKey, error) {
	return ecdh.P256().GenerateKey(rand.Reader)
}

// SealV3 encrypts a reply to a requester's public key under protocol v3: a
// fresh ephemeral P-256 key agrees a shared secret with pubR, HKDF-SHA-256
// derives the AES-256-GCM key, and the ephemeral public key is prefixed to
// the ciphertext as the authenticated header.
//
// Parameters:
//   - pubR: the requester's uncompressed P-256 public key (65 bytes).
//   - plaintext: bytes to encrypt (raw or a GONE2 envelope).
//
// Returns the nonce and blob (pubE || ciphertext || tag), or ErrInvalidKey,
// or an RNG error.
func SealV3(pubR, plaintext []byte) (nonce, blob []byte, err error) {
	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return sealV3With(rand.Reader, eph, pubR, plaintext)
}

// sealV3With is SealV3 with an injectable ephemeral key and nonce source so
// vectors can be reproduced exactly.
//
// Parameters:
//   - r: nonce randomness source.
//   - eph: ephemeral P-256 private key.
//   - pubR: the requester's public key.
//   - plaintext: bytes to encrypt.
//
// Returns the nonce and blob, or ErrInvalidKey or the read error.
func sealV3With(r io.Reader, eph *ecdh.PrivateKey, pubR, plaintext []byte) (nonce, blob []byte, err error) {
	peer, err := ecdh.P256().NewPublicKey(pubR)
	if err != nil {
		return nil, nil, ErrInvalidKey
	}
	nonce = make([]byte, domain.NonceSize)
	if _, err := io.ReadFull(r, nonce); err != nil {
		return nil, nil, err
	}
	hdr := eph.PublicKey().Bytes()
	aead, err := v3AEAD(eph, peer, hdr, pubR)
	if err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(hdr, nonce, plaintext, v3AAD(hdr)), nil
}

// OpenV3 authenticates and decrypts a v3 reply blob with the requester's
// private key. Every failure, including a short blob, an invalid ephemeral
// point, or a failed authentication, returns the single generic ErrDecrypt.
//
// Parameters:
//   - priv: the requester's P-256 private key.
//   - nonce: NonceSize-byte nonce from X-Gone-Nonce.
//   - blob: pubE || ciphertext || tag as stored by the server.
//
// Returns the plaintext or ErrDecrypt.
func OpenV3(priv *ecdh.PrivateKey, nonce, blob []byte) ([]byte, error) {
	if priv == nil || priv.Curve() != ecdh.P256() || len(nonce) != domain.NonceSize ||
		len(blob) < domain.V3HeaderSize+domain.TagSize {
		return nil, ErrDecrypt
	}
	hdr := blob[:domain.V3HeaderSize]
	peer, err := ecdh.P256().NewPublicKey(hdr)
	if err != nil {
		return nil, ErrDecrypt
	}
	aead, err := v3AEAD(priv, peer, hdr, priv.PublicKey().Bytes())
	if err != nil {
		return nil, ErrDecrypt
	}
	pt, err := aead.Open(nil, nonce, blob[domain.V3HeaderSize:], v3AAD(hdr))
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// deriveV3Key runs ECDH between priv and peer and expands the shared secret
// with HKDF-SHA-256: HKDF(IKM = Z, salt = empty,
// info = V3HKDFInfo || pubE || pubR).
//
// Parameters:
//   - priv: own private key (ephemeral when sealing, requester when opening).
//   - peer: the other side's public key.
//   - pubE: the ephemeral public key bytes.
//   - pubR: the requester's public key bytes.
//
// Returns the 32-byte AEAD key or an ECDH/KDF error.
func deriveV3Key(priv *ecdh.PrivateKey, peer *ecdh.PublicKey, pubE, pubR []byte) ([]byte, error) {
	z, err := priv.ECDH(peer)
	if err != nil {
		return nil, err
	}
	defer clear(z)
	return hkdf.Key(sha256.New, z, nil, v3Info(pubE, pubR), domain.KeySize)
}

// v3AEAD derives the v3 key and builds its AES-256-GCM AEAD, wiping the key
// before returning.
//
// Parameters:
//   - priv: own private key.
//   - peer: the other side's public key.
//   - pubE: the ephemeral public key bytes.
//   - pubR: the requester's public key bytes.
//
// Returns the AEAD or an ECDH/KDF/cipher error.
func v3AEAD(priv *ecdh.PrivateKey, peer *ecdh.PublicKey, pubE, pubR []byte) (cipher.AEAD, error) {
	k, err := deriveV3Key(priv, peer, pubE, pubR)
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

// v3Info returns the HKDF info string: "gone:v3 aead key" || pubE || pubR.
//
// Parameters:
//   - pubE: the ephemeral public key bytes.
//   - pubR: the requester's public key bytes.
//
// Returns the info string.
func v3Info(pubE, pubR []byte) string {
	info := make([]byte, 0, len(domain.V3HKDFInfo)+len(pubE)+len(pubR))
	return string(append(append(append(info, domain.V3HKDFInfo...), pubE...), pubR...))
}

// v3AAD returns the v3 associated data: "gone:v3" || hdr.
//
// Parameters:
//   - hdr: the raw V3HeaderSize-byte header (pubE).
//
// Returns a fresh AAD slice.
func v3AAD(hdr []byte) []byte {
	aad := make([]byte, 0, len(domain.AADv3)+len(hdr))
	return append(append(aad, domain.AADv3...), hdr...)
}
