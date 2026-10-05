package envelope

import (
	"bytes"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

type aeadCase struct {
	Name       string `json:"name"`
	Key        string `json:"key"`
	Nonce      string `json:"nonce"`
	Plaintext  string `json:"plaintext,omitempty"`
	Ciphertext string `json:"ciphertext"`
	Error      string `json:"error,omitempty"`
}

type aeadVectors struct {
	Description string     `json:"description"`
	AAD         string     `json:"aad"`
	Cases       []aeadCase `json:"cases"`
}

// buildAEAD constructs deterministic protocol v1 AEAD vectors.
//
// Parameters:
//   - t: the test handle.
//
// Returns:
//   - aeadVectors: the positive and negative AEAD vector cases.
func buildAEAD(t *testing.T) aeadVectors {
	t.Helper()
	key, nonce := seq(0, domain.KeySize), seq(0xa0, domain.NonceSize)
	pts := []struct {
		name string
		pt   []byte
	}{
		{"empty", nil},
		{"text", []byte("gone vector")},
		{"two blocks plus", seq(0x40, 33)},
	}
	v := aeadVectors{Description: "AES-256-GCM v1; hex fields; AAD is UTF-8 text", AAD: domain.AADv1}
	var ct []byte
	for _, p := range pts {
		gotNonce, c, err := sealFrom(bytes.NewReader(nonce), key, p.pt)
		if err != nil || !bytes.Equal(gotNonce, nonce) {
			t.Fatalf("seal %s: %v", p.name, err)
		}
		ct = c
		v.Cases = append(v.Cases, aeadCase{Name: p.name, Key: hx(key), Nonce: hx(nonce), Plaintext: hx(p.pt), Ciphertext: hx(c)})
	}
	return appendAEADNegatives(v, key, nonce, ct)
}

// appendAEADNegatives appends corrupted protocol v1 AEAD cases.
//
// Parameters:
//   - v: the vectors to extend.
//   - key: the baseline link key.
//   - nonce: the baseline nonce.
//   - ct: the baseline ciphertext.
//
// Returns:
//   - aeadVectors: v with negative cases appended.
func appendAEADNegatives(v aeadVectors, key []byte, nonce []byte, ct []byte) aeadVectors {
	other := seq(1, domain.KeySize)
	noAAD, _ := newAEAD(key)
	neg := []struct {
		name           string
		key, nonce, ct []byte
	}{
		{"flipped tag bit", key, nonce, flippedByte(ct, len(ct)-1)},
		{"flipped ciphertext bit", key, nonce, flippedByte(ct, 0)},
		{"wrong aad", key, nonce, noAAD.Seal(nil, nonce, seq(0x40, 33), nil)},
		{"wrong key", other, nonce, ct},
		{"short nonce", key, nonce[:11], ct},
		{"31-byte key", key[:31], nonce, ct},
		{"truncated tag", key, nonce, ct[:domain.TagSize-1]},
	}
	for _, n := range neg {
		_, err := Open(n.key, n.nonce, n.ct)
		v.Cases = append(v.Cases, aeadCase{Name: n.name, Key: hx(n.key), Nonce: hx(n.nonce), Ciphertext: hx(n.ct), Error: errCode(err)})
	}
	return v
}

// flippedByte returns a copy of b with one bit flipped at index i.
//
// Parameters:
//   - b: the source bytes.
//   - i: the byte index to mutate.
//
// Returns:
//   - []byte: the mutated copy.
func flippedByte(b []byte, i int) []byte {
	c := bytes.Clone(b)
	c[i] ^= 0x01
	return c
}
