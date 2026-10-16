package integration_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// v2Link and v2Nonce are the fixed link key and nonce of the v2 cases.
var (
	v2Link  = bytes.Repeat([]byte{0x42}, domain.KeySize)
	v2Nonce = bytes.Repeat([]byte{0x24}, domain.NonceSize)
)

// newGCM builds AES-256-GCM for key.
//
// Parameters:
//   - t: the test.
//   - key: 32-byte key.
//
// Returns the AEAD.
func newGCM(t *testing.T, key []byte) cipher.AEAD {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return aead
}

// sealV2Iter seals pt under protocol v2 with any iteration count, written
// out from docs/protocol.md §4.2 (envelope.SealV2 always uses 600,000).
//
// Parameters:
//   - t: the test.
//   - pass: the passphrase, already NFC.
//   - iter: PBKDF2 iterations written to the header.
//   - pt: plaintext.
//
// Returns the blob, sealed with v2Link and v2Nonce.
func sealV2Iter(t *testing.T, pass string, iter uint32, pt []byte) []byte {
	t.Helper()
	salt := bytes.Repeat([]byte{0x5a}, domain.V2SaltSize)
	hdr := binary.BigEndian.AppendUint32([]byte{domain.KDFPBKDF2SHA256}, iter)
	hdr = append(hdr, salt...)
	pw, err := pbkdf2.Key(sha256.New, pass, salt, int(iter), domain.KeySize)
	if err != nil {
		t.Fatal(err)
	}
	k, err := hkdf.Key(sha256.New, append(bytes.Clone(v2Link), pw...), nil, domain.V2HKDFInfo, domain.KeySize)
	if err != nil {
		t.Fatal(err)
	}
	return newGCM(t, k).Seal(hdr, v2Nonce, pt, append([]byte(domain.AADv2), hdr...))
}

// sealV2 seals pt like envelope.SealV2, with 600,000 iterations and the
// passphrase normalized to NFC, under v2Link and v2Nonce.
//
// Parameters:
//   - t: the test.
//   - pass: the passphrase.
//   - pt: plaintext.
//
// Returns the blob.
func sealV2(t *testing.T, pass string, pt []byte) []byte {
	t.Helper()
	return sealV2Iter(t, norm.NFC.String(pass), domain.V2Iterations, pt)
}

// v2Case opens blob with OpenV2 and with cryptoV2.js decryptV2 (the
// bridge's open op).
//
// Parameters:
//   - name: case name.
//   - pass: the passphrase given to both readers.
//   - blob: the v2 blob.
//
// Returns the case.
func v2Case(name, pass string, blob []byte) diffCase {
	return diffCase{
		name: name,
		goV: func(*testing.T) verdict {
			return goPayload(envelope.OpenV2(v2Link, pass, v2Nonce, blob))
		},
		js: diffReq{
			Op: "open", Version: domain.ProtocolV2, Key: base64.RawURLEncoding.EncodeToString(v2Link),
			Nonce: v2Nonce, Body: blob, Passphrase: pass,
		},
		jsValue: jsPayload,
	}
}

// withIter returns a copy of blob with another iteration count.
//
// Parameters:
//   - blob: a v2 blob.
//   - iter: the iteration count.
//
// Returns the edited copy.
func withIter(blob []byte, iter uint32) []byte {
	b := bytes.Clone(blob)
	binary.BigEndian.PutUint32(b[1:5], iter)
	return b
}

// withByte returns a copy of b with b[i] set to v.
//
// Parameters:
//   - b: bytes.
//   - i: index.
//   - v: new value.
//
// Returns the edited copy.
func withByte(b []byte, i int, v byte) []byte {
	c := bytes.Clone(b)
	c[i] = v
	return c
}

// v2Cases are v2 header and passphrase boundaries (docs/protocol.md §4.2).
//
// Parameters:
//   - t: the test.
//
// Returns the cases.
func v2Cases(t *testing.T) []diffCase {
	const pass = "correct horse"
	base := sealV2(t, pass, []byte("v2 differential"))
	composed, decomposed := "caf\u00e9", "cafe\u0301"
	long := strings.Repeat("a", domain.V2MaxPassphraseBytes)
	// U+0958 has no composed form: NFC turns its 3 bytes into 6.
	expands := strings.Repeat("\u0958", 200)
	return []diffCase{
		v2Case("iterations 600000", pass, base),
		v2Case("iterations 599999", pass, withIter(base, domain.V2MinIterations-1)),
		v2Case("iterations 5000000", pass, sealV2Iter(t, pass, domain.V2MaxIterations, []byte("max"))),
		v2Case("iterations 5000001", pass, withIter(base, domain.V2MaxIterations+1)),
		v2Case("iterations 0", pass, withIter(base, 0)),
		v2Case("iterations 2^32-1", pass, withIter(base, 1<<32-1)),
		v2Case("kdf id 0x00", pass, withByte(base, 0, 0x00)),
		v2Case("kdf id 0x02", pass, withByte(base, 0, 0x02)),
		v2Case("body 36 bytes", pass, base[:domain.V2HeaderSize+domain.TagSize-1]),
		v2Case("body 37 bytes", pass, sealV2(t, pass, nil)),
		v2Case("passphrase 1024 bytes", long, sealV2(t, long, []byte("long"))),
		v2Case("passphrase 1025 bytes", long+"a", base),
		v2Case("passphrase 1026 bytes NFC to 684", strings.Repeat("e\u0301", 342), sealV2(t, strings.Repeat("\u00e9", 342), []byte("nfc"))),
		v2Case("passphrase 600 bytes NFC to 1200", expands, base),
		v2Case("passphrase NFD opens NFC seal", decomposed, sealV2(t, composed, []byte("nfc"))),
		v2Case("passphrase NFC opens seal from NFD input", composed, sealV2(t, decomposed, []byte("nfc"))),
		v2Case("passphrase empty", "", base),
		v2Case("passphrase wrong", pass+"!", base),
	}
}

// TestDifferentialV2 feeds the same v2 blobs and passphrases to OpenV2 and
// to cryptoV2.js and fails when they disagree.
//
// Parameters:
//   - t: the test.
func TestDifferentialV2(t *testing.T) {
	runDiff(t, v2Cases(t))
}
