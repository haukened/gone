package envelope

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/haukened/gone/internal/domain"
)

// aeadV2Case is one protocol v2 vector. The passphrase is hex-encoded UTF-8
// so the NFD case survives editors and JSON tooling byte-for-byte; pw and
// aead_key are the PBKDF2 and HKDF intermediates for debugging ports.
type aeadV2Case struct {
	Name       string `json:"name"`
	Key        string `json:"key"`
	Passphrase string `json:"passphrase"`
	Nonce      string `json:"nonce"`
	Salt       string `json:"salt,omitempty"`
	Iterations uint32 `json:"iterations,omitempty"`
	Plaintext  string `json:"plaintext,omitempty"`
	PW         string `json:"pw,omitempty"`
	AEADKey    string `json:"aead_key,omitempty"`
	Blob       string `json:"blob"`
	Error      string `json:"error,omitempty"`
}

type aeadV2Vectors struct {
	Description string       `json:"description"`
	AAD         string       `json:"aad"`
	HKDFInfo    string       `json:"hkdf_info"`
	Cases       []aeadV2Case `json:"cases"`
}

// buildAEADV2 seals the positive v2 cases deterministically, records the
// derivation intermediates, and appends negative cases derived from the
// first blob.
func buildAEADV2(t *testing.T) aeadV2Vectors {
	key, salt, nonce := seq(0, domain.KeySize), seq(0x10, domain.V2SaltSize), seq(0xa0, domain.NonceSize)
	gone2, err := Pack(Payload{Message: []byte("see attached"), Files: []File{{Name: "a.txt", Type: "text/plain", Data: []byte("hi")}}})
	if err != nil {
		t.Fatal(err)
	}
	nfc := "P\u00e4ssw\u00f6rd caf\u00e9 \u2615"
	pts := []struct {
		name, pass string
		pt         []byte
	}{
		{"ascii passphrase", "correct horse battery staple", []byte("gone v2 vector")},
		{"unicode NFC passphrase", nfc, []byte("unicode")},
		{"NFD input matches NFC", norm.NFD.String(nfc), []byte("unicode")},
		{"GONE2 plaintext", "FrostCanalBloomTrickRuby", gone2},
	}
	v := aeadV2Vectors{
		Description: "Protocol v2 (docs/protocol.md 4.2); hex fields; passphrase is hex UTF-8 before NFC; blob = hdr || ct || tag",
		AAD:         domain.AADv2,
		HKDFInfo:    domain.V2HKDFInfo,
	}
	for _, p := range pts {
		v.Cases = append(v.Cases, sealV2Vector(t, p.name, key, p.pass, salt, nonce, p.pt))
	}
	return appendAEADV2Negatives(v, key, nonce, v.Cases[0])
}

// sealV2Vector produces one positive v2 case with its intermediates.
func sealV2Vector(t *testing.T, name string, key []byte, pass string, salt, nonce, pt []byte) aeadV2Case {
	gotNonce, blob, err := sealV2From(bytes.NewReader(append(bytes.Clone(salt), nonce...)), key, pass, pt)
	if err != nil || !bytes.Equal(gotNonce, nonce) {
		t.Fatalf("seal %s: %v", name, err)
	}
	p, _ := normalizePassphrase(pass)
	pw, err := derivePassphraseKey(p, salt, domain.V2Iterations)
	if err != nil {
		t.Fatal(err)
	}
	k, err := deriveV2Key(key, pw)
	if err != nil {
		t.Fatal(err)
	}
	return aeadV2Case{
		Name: name, Key: hx(key), Passphrase: hx([]byte(pass)), Nonce: hx(nonce),
		Salt: hx(salt), Iterations: domain.V2Iterations, Plaintext: hx(pt),
		PW: hx(pw), AEADKey: hx(k), Blob: hx(blob),
	}
}

// appendAEADV2Negatives records the reader's verdict on corrupted inputs:
// "decrypt" (retryable) versus "malformed" (structural, not retryable).
func appendAEADV2Negatives(v aeadV2Vectors, key, nonce []byte, base aeadV2Case) aeadV2Vectors {
	blob := hexBytes(base.Blob)
	pass := string(hexBytes(base.Passphrase))
	mut := func(f func([]byte)) []byte { b := bytes.Clone(blob); f(b); return b }
	iter := func(n uint32) []byte { return mut(func(b []byte) { binary.BigEndian.PutUint32(b[1:5], n) }) }
	neg := []struct {
		name string
		key  []byte
		pass string
		blob []byte
	}{
		{"wrong passphrase", key, pass + "!", blob},
		{"wrong link key", seq(1, domain.KeySize), pass, blob},
		{"flipped salt bit", key, pass, mut(func(b []byte) { b[5] ^= 0x01 })},
		{"iterations below minimum", key, pass, iter(domain.V2MinIterations - 1)},
		{"iterations above maximum", key, pass, iter(domain.V2MaxIterations + 1)},
		{"unknown kdf id", key, pass, mut(func(b []byte) { b[0] = 0x02 })},
		{"truncated blob", key, pass, blob[:domain.V2HeaderSize+domain.TagSize-1]},
		{"empty passphrase", key, "", blob},
	}
	for _, n := range neg {
		_, err := OpenV2(n.key, n.pass, nonce, n.blob)
		v.Cases = append(v.Cases, aeadV2Case{
			Name: n.name, Key: hx(n.key), Passphrase: hx([]byte(n.pass)), Nonce: hx(nonce),
			Blob: hx(n.blob), Error: errCode(err),
		})
	}
	return v
}

// hexBytes decodes a hex string produced by hx; it panics on bad input
// because every caller passes builder output.
func hexBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// fragmentV2Inputs are the v2 fragment cases; v3 marks the next
// unsupported version.
func fragmentV2Inputs() []string {
	k := domain.EncodeB64URL(seq(0, domain.KeySize))
	zeros := strings.Repeat("A", 43)
	return []string{
		"v2:" + k, "v2:" + zeros, "v2:", "v02:" + k, "V2:" + k, "v2:" + k + "=",
		"v2:" + zeros[:42], "v2:" + domain.EncodeB64URL(seq(0, 33)), "v3:" + k,
	}
}
