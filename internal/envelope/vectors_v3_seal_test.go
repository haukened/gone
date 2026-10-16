package envelope

import (
	"bytes"
	"crypto/ecdh"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

// aeadV3SealError is a v3 sealing vector: a requester public key that every
// sender must refuse before encrypting (docs/protocol.md §4.3).
type aeadV3SealError struct {
	Name      string `json:"name"`
	PubR      string `json:"pub_r"`
	Plaintext string `json:"plaintext"`
	Error     string `json:"error"`
}

// hybridPoint returns the SEC 1 hybrid encoding (0x06 or 0x07 || X || Y) of
// an uncompressed P-256 point. Some WebCrypto implementations import it;
// docs/protocol.md §4.3 allows only the uncompressed form.
//
// Parameters:
//   - uncompressed: a 65-byte 0x04 point.
//
// Returns the hybrid encoding.
func hybridPoint(uncompressed []byte) []byte {
	h := bytes.Clone(uncompressed)
	h[0] = 0x06 | uncompressed[64]&1
	return h
}

// sealV3Hybrid seals like sealV3With but writes the ephemeral point in its
// hybrid encoding, binding those bytes into the HKDF info and the AAD. A
// reader that accepted the encoding would open the result.
//
// Parameters:
//   - t: the test handle.
//   - eph: the ephemeral private key.
//   - peer: the requester public key.
//   - nonce: fixed nonce.
//   - pt: plaintext.
//
// Returns the blob: hybrid pubE || ct || tag.
func sealV3Hybrid(t *testing.T, eph *ecdh.PrivateKey, peer *ecdh.PublicKey, nonce, pt []byte) []byte {
	hdr := hybridPoint(eph.PublicKey().Bytes())
	aead, err := v3AEAD(eph, peer, hdr, peer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return aead.Seal(hdr, nonce, pt, v3AAD(hdr))
}

// buildAEADV3SealErrors records SealV3's verdict on requester keys that are
// not uncompressed points on the curve.
//
// Parameters:
//   - t: the test handle.
//   - pubR: a valid uncompressed requester key to derive the bad ones from.
//
// Returns the cases.
func buildAEADV3SealErrors(t *testing.T, pubR []byte) []aeadV3SealError {
	offCurve := bytes.Clone(pubR)
	offCurve[64] ^= 0x01
	keys := []struct {
		name string
		key  []byte
	}{
		{"requester point not on curve", offCurve},
		{"compressed requester point", append([]byte{0x02 | pubR[64]&1}, pubR[1:33]...)},
		{"hybrid requester point", hybridPoint(pubR)},
		{"requester point at infinity", []byte{0x00}},
		{"all-zero requester point", make([]byte, domain.V3PublicKeySize)},
	}
	pt := []byte("never sealed")
	out := make([]aeadV3SealError, len(keys))
	for i, k := range keys {
		_, _, err := SealV3(k.key, pt)
		if err == nil {
			t.Fatalf("%s: sealed", k.name)
		}
		out[i] = aeadV3SealError{Name: k.name, PubR: hx(k.key), Plaintext: hx(pt), Error: errCode(err)}
	}
	return out
}
