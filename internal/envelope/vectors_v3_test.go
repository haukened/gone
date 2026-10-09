package envelope

import (
	"bytes"
	"crypto/ecdh"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

// aeadV3Case is one protocol v3 vector. Private keys are raw P-256 scalars;
// shared and aead_key are the ECDH and HKDF intermediates for debugging ports.
type aeadV3Case struct {
	Name      string `json:"name"`
	PrivR     string `json:"priv_r"`
	PubR      string `json:"pub_r"`
	PrivE     string `json:"priv_e,omitempty"`
	PubE      string `json:"pub_e,omitempty"`
	Nonce     string `json:"nonce"`
	Plaintext string `json:"plaintext,omitempty"`
	Shared    string `json:"shared,omitempty"`
	AEADKey   string `json:"aead_key,omitempty"`
	Blob      string `json:"blob"`
	Error     string `json:"error,omitempty"`
}

type aeadV3Vectors struct {
	Description string       `json:"description"`
	AAD         string       `json:"aad"`
	HKDFInfo    string       `json:"hkdf_info"`
	Cases       []aeadV3Case `json:"cases"`
}

// p256Key builds a deterministic P-256 private key from a raw scalar.
//
// Parameters:
//   - t: the test handle.
//   - d: 32-byte scalar.
//
// Returns the private key.
func p256Key(t *testing.T, d []byte) *ecdh.PrivateKey {
	t.Helper()
	k, err := ecdh.P256().NewPrivateKey(d)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// buildAEADV3 seals the positive v3 cases deterministically and appends
// negative cases derived from the first blob.
//
// Parameters:
//   - t: the test handle.
//
// Returns the v3 vectors.
func buildAEADV3(t *testing.T) aeadV3Vectors {
	privR := p256Key(t, seq(0x01, 32))
	nonce := seq(0xb0, domain.NonceSize)
	gone2, err := Pack(Payload{Message: []byte("here you go"), Files: []File{{Name: "id.txt", Type: "text/plain", Data: []byte("ok")}}})
	if err != nil {
		t.Fatal(err)
	}
	pts := []struct {
		name string
		d    []byte
		pt   []byte
	}{
		{"text reply", seq(0x40, 32), []byte("gone v3 vector")},
		{"GONE2 reply", seq(0x60, 32), gone2},
		{"empty plaintext", seq(0x20, 32), nil},
	}
	v := aeadV3Vectors{
		Description: "Protocol v3 (docs/protocol.md 4.3); hex fields; private keys are raw P-256 scalars; blob = pubE || ct || tag",
		AAD:         domain.AADv3,
		HKDFInfo:    domain.V3HKDFInfo,
	}
	for _, p := range pts {
		v.Cases = append(v.Cases, sealV3Vector(t, p.name, privR, p256Key(t, p.d), nonce, p.pt))
	}
	return appendAEADV3Negatives(t, v, privR, nonce, v.Cases[0])
}

// sealV3Vector produces one positive v3 case with its intermediates.
//
// Parameters:
//   - t: the test handle.
//   - name: case name.
//   - privR: requester key.
//   - eph: ephemeral key.
//   - nonce: fixed nonce.
//   - pt: plaintext.
//
// Returns the case.
func sealV3Vector(t *testing.T, name string, privR, eph *ecdh.PrivateKey, nonce, pt []byte) aeadV3Case {
	pubR := privR.PublicKey().Bytes()
	gotNonce, blob, err := sealV3With(bytes.NewReader(nonce), eph, pubR, pt)
	if err != nil || !bytes.Equal(gotNonce, nonce) {
		t.Fatalf("seal %s: %v", name, err)
	}
	z, err := eph.ECDH(privR.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	k, err := deriveV3Key(eph, privR.PublicKey(), eph.PublicKey().Bytes(), pubR)
	if err != nil {
		t.Fatal(err)
	}
	return aeadV3Case{
		Name: name, PrivR: hx(privR.Bytes()), PubR: hx(pubR), PrivE: hx(eph.Bytes()),
		PubE: hx(eph.PublicKey().Bytes()), Nonce: hx(nonce), Plaintext: hx(pt),
		Shared: hx(z), AEADKey: hx(k), Blob: hx(blob),
	}
}

// appendAEADV3Negatives records the reader's verdict on corrupted inputs.
// Every v3 failure is the generic "decrypt".
//
// Parameters:
//   - t: the test handle.
//   - v: vectors so far.
//   - privR: requester key.
//   - nonce: the nonce used by base.
//   - base: the first positive case.
//
// Returns v with the negative cases appended.
func appendAEADV3Negatives(t *testing.T, v aeadV3Vectors, privR *ecdh.PrivateKey, nonce []byte, base aeadV3Case) aeadV3Vectors {
	blob := hexBytes(base.Blob)
	mut := func(f func([]byte)) []byte { b := bytes.Clone(blob); f(b); return b }
	other := p256Key(t, seq(0x02, 32))
	neg := []struct {
		name  string
		key   *ecdh.PrivateKey
		nonce []byte
		blob  []byte
	}{
		{"wrong requester key", other, nonce, blob},
		{"flipped ciphertext bit", privR, nonce, mut(func(b []byte) { b[len(b)-1] ^= 0x01 })},
		{"ephemeral point not on curve", privR, nonce, mut(func(b []byte) { b[64] ^= 0x01 })},
		{"compressed ephemeral point", privR, nonce, mut(func(b []byte) { b[0] = 0x02 })},
		{"wrong nonce", privR, seq(0xb1, domain.NonceSize), blob},
		{"truncated blob", privR, nonce, blob[:domain.V3HeaderSize+domain.TagSize-1]},
	}
	for _, n := range neg {
		_, err := OpenV3(n.key, n.nonce, n.blob)
		v.Cases = append(v.Cases, aeadV3Case{
			Name: n.name, PrivR: hx(n.key.Bytes()), PubR: hx(n.key.PublicKey().Bytes()),
			Nonce: hx(n.nonce), Blob: hx(n.blob), Error: errCode(err),
		})
	}
	return v
}

// replyFragmentCase is one reply-fragment parsing vector.
type replyFragmentCase struct {
	Input string `json:"input"`
	Key   string `json:"key,omitempty"`
	Fill  string `json:"fill,omitempty"`
	Error string `json:"error,omitempty"`
}

// fragmentV3Inputs are the reply fragment cases.
//
// Parameters:
//   - t: the test handle.
//
// Returns the inputs.
func fragmentV3Inputs(t *testing.T) []string {
	pub := domain.EncodeB64URL(p256Key(t, seq(0x01, 32)).PublicKey().Bytes())
	fill := strings.Repeat("A", 43)
	compressed := domain.EncodeB64URL(append([]byte{0x02}, seq(0, 64)...))
	return []string{
		"v3:" + pub + "." + fill,
		"v3:" + pub,
		"v3:" + pub + ".",
		"v3:" + pub + "." + fill[:42],
		"v3:" + pub + "." + fill + "=",
		"v3:" + pub[:86] + "." + fill,
		"v3:" + compressed + "." + fill,
		"v3:." + fill,
		"v03:" + pub + "." + fill,
		"V3:" + pub + "." + fill,
		"v3:" + pub + "." + fill + "." + fill,
		"v3:" + pub + "%2E" + fill,
		"v1:" + pub + "." + fill,
		"v4:" + pub + "." + fill,
		"v3:" + strings.Repeat("A", 600),
	}
}

// buildReplyFragments parses each input and records the result.
//
// Parameters:
//   - ins: fragment inputs.
//
// Returns the cases.
func buildReplyFragments(ins []string) []replyFragmentCase {
	out := make([]replyFragmentCase, len(ins))
	for i, s := range ins {
		c := replyFragmentCase{Input: s}
		f, err := ParseReplyFragment(s)
		if err != nil {
			c.Error = errCode(err)
		} else {
			c.Key, c.Fill = hx(f.PublicKey), f.Fill.String()
		}
		out[i] = c
	}
	return out
}
