package envelope

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

// v2Fixture seals one plaintext so tests can reuse the expensive PBKDF2 run.
//
// Parameters:
//   - t: the test handle.
//
// Returns:
//   - key: symmetric link key used for sealing.
//   - nonce: random AEAD nonce returned by SealV2.
//   - blob: sealed v2 blob returned by SealV2.
func v2Fixture(t *testing.T) (key, nonce, blob []byte) {
	t.Helper()
	key = bytes.Repeat([]byte{7}, domain.KeySize)
	nonce, blob, err := SealV2(key, "CorrectHorseBattery", []byte("hello v2"))
	if err != nil {
		t.Fatalf("SealV2: %v", err)
	}
	return key, nonce, blob
}

func TestSealV2Layout(t *testing.T) {
	key, nonce, blob := v2Fixture(t)
	requireV2Nonce(t, nonce)
	requireV2BlobLayout(t, blob)
	requireV2Opens(t, key, nonce, blob)
	requireV2SaltChanges(t, key, blob)
}

// requireV2Nonce verifies a v2 nonce has the expected size.
//
// Parameters:
//   - t: the test handle.
//   - nonce: nonce returned by SealV2.
func requireV2Nonce(t *testing.T, nonce []byte) {
	t.Helper()
	if len(nonce) != domain.NonceSize {
		t.Fatalf("nonce = %d bytes", len(nonce))
	}
}

// requireV2BlobLayout verifies the v2 blob length and header fields.
//
// Parameters:
//   - t: the test handle.
//   - blob: sealed v2 blob.
func requireV2BlobLayout(t *testing.T, blob []byte) {
	t.Helper()
	if want := domain.V2HeaderSize + len("hello v2") + domain.TagSize; len(blob) != want {
		t.Fatalf("blob = %d bytes, want %d", len(blob), want)
	}
	if blob[0] != domain.KDFPBKDF2SHA256 {
		t.Fatalf("kdf = %x", blob[0])
	}
	if binary.BigEndian.Uint32(blob[1:5]) != domain.V2Iterations {
		t.Fatalf("iterations = %x", blob[1:5])
	}
}

// requireV2Opens verifies the v2 blob decrypts to the expected plaintext.
//
// Parameters:
//   - t: the test handle.
//   - key: symmetric link key for opening.
//   - nonce: AEAD nonce for opening.
//   - blob: sealed v2 blob.
func requireV2Opens(t *testing.T, key []byte, nonce []byte, blob []byte) {
	t.Helper()
	got, err := OpenV2(key, "CorrectHorseBattery", nonce, blob)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello v2" {
		t.Fatalf("OpenV2 = %q", got)
	}
}

// requireV2SaltChanges verifies independent seals use distinct salts.
//
// Parameters:
//   - t: the test handle.
//   - key: symmetric link key for sealing.
//   - blob: first sealed v2 blob.
func requireV2SaltChanges(t *testing.T, key []byte, blob []byte) {
	t.Helper()
	_, blob2, err := SealV2(key, "CorrectHorseBattery", []byte("hello v2"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(blob[5:21], blob2[5:21]) {
		t.Fatal("salt reused across seals")
	}
}

func TestOpenV2Failures(t *testing.T) {
	key, nonce, blob := v2Fixture(t)
	mut := func(i int, f func([]byte)) []byte {
		b := bytes.Clone(blob)
		f(b[i:])
		return b
	}
	setIter := func(n uint32) []byte {
		return mut(1, func(b []byte) { binary.BigEndian.PutUint32(b, n) })
	}
	otherKey := bytes.Repeat([]byte{8}, domain.KeySize)
	cases := []struct {
		name  string
		key   []byte
		pass  string
		nonce []byte
		blob  []byte
		want  error
	}{
		{"wrong passphrase", key, "CorrectHorseBatterz", nonce, blob, ErrDecrypt},
		{"wrong link key", otherKey, "CorrectHorseBattery", nonce, blob, ErrDecrypt},
		{"flipped salt", key, "CorrectHorseBattery", nonce, mut(10, func(b []byte) { b[0] ^= 1 }), ErrDecrypt},
		{"flipped ciphertext", key, "CorrectHorseBattery", nonce, mut(domain.V2HeaderSize, func(b []byte) { b[0] ^= 1 }), ErrDecrypt},
		{"iterations changed in range", key, "CorrectHorseBattery", nonce, setIter(domain.V2Iterations + 1), ErrDecrypt},
		{"iterations below min", key, "CorrectHorseBattery", nonce, setIter(domain.V2MinIterations - 1), ErrMalformed},
		{"iterations above max", key, "CorrectHorseBattery", nonce, setIter(domain.V2MaxIterations + 1), ErrMalformed},
		{"unknown kdf", key, "CorrectHorseBattery", nonce, mut(0, func(b []byte) { b[0] = 0x02 }), ErrMalformed},
		{"short blob", key, "CorrectHorseBattery", nonce, blob[:domain.V2HeaderSize+domain.TagSize-1], ErrMalformed},
		{"empty blob", key, "CorrectHorseBattery", nonce, nil, ErrMalformed},
		{"short key", key[:16], "CorrectHorseBattery", nonce, blob, ErrDecrypt},
		{"short nonce", key, "CorrectHorseBattery", nonce[:8], blob, ErrDecrypt},
		{"empty passphrase", key, "", nonce, blob, ErrInvalidPassphrase},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := OpenV2(c.key, c.pass, c.nonce, c.blob); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestSealV2Errors(t *testing.T) {
	key := bytes.Repeat([]byte{7}, domain.KeySize)
	cases := []struct {
		name string
		rng  []byte
		key  []byte
		pass string
		want error
	}{
		{"short key", nil, key[:31], "pass", ErrInvalidKey},
		{"empty passphrase", nil, key, "", ErrInvalidPassphrase},
		{"invalid utf8", nil, key, "\xff\xfe", ErrInvalidPassphrase},
		{"too long", nil, key, strings.Repeat("a", domain.V2MaxPassphraseBytes+1), ErrInvalidPassphrase},
		{"salt read fails", make([]byte, 4), key, "pass", nil},
		{"nonce read fails", make([]byte, domain.V2SaltSize+4), key, "pass", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := sealV2From(bytes.NewReader(c.rng), c.key, c.pass, []byte("x"))
			if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestNormalizePassphrase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{"ascii kept", "  Spaces Kept  ", "  Spaces Kept  ", nil},
		{"composed", "caf\u00e9", "caf\u00e9", nil},
		{"decomposed to composed", "cafe\u0301", "caf\u00e9", nil},
		{"exactly max", strings.Repeat("a", domain.V2MaxPassphraseBytes), strings.Repeat("a", domain.V2MaxPassphraseBytes), nil},
		{"over max", strings.Repeat("a", domain.V2MaxPassphraseBytes+1), "", ErrInvalidPassphrase},
		{"multibyte over max", strings.Repeat("\u00e9", domain.V2MaxPassphraseBytes/2+1), "", ErrInvalidPassphrase},
		{"empty", "", "", ErrInvalidPassphrase},
		{"invalid utf8", "a\xc3", "", ErrInvalidPassphrase},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizePassphrase(c.in)
			if !errors.Is(err, c.err) || string(got) != c.want {
				t.Fatalf("normalizePassphrase = %q, %v", got, err)
			}
		})
	}
}

func TestOpenV2NFCEquivalence(t *testing.T) {
	key := bytes.Repeat([]byte{9}, domain.KeySize)
	nonce, blob, err := SealV2(key, "caf\u00e9 au lait", []byte("pt"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := OpenV2(key, "cafe\u0301 au lait", nonce, blob)
	if err != nil || string(got) != "pt" {
		t.Fatalf("OpenV2(NFD) = %q, %v", got, err)
	}
}

func TestParseV2HeaderBounds(t *testing.T) {
	salt := bytes.Repeat([]byte{1}, domain.V2SaltSize)
	tail := make([]byte, domain.TagSize)
	for _, iter := range []uint32{domain.V2MinIterations, domain.V2MaxIterations} {
		blob := append(buildV2Header(iter, salt), tail...)
		h, err := parseV2Header(blob)
		if err != nil || h.iterations != iter || !bytes.Equal(h.salt, salt) || len(h.raw) != domain.V2HeaderSize {
			t.Fatalf("parseV2Header(%d) = %+v, %v", iter, h, err)
		}
	}
}

func TestDeriveV2KeyDistinct(t *testing.T) {
	a, _ := deriveV2Key(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	b, _ := deriveV2Key(bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{1}, 32))
	if len(a) != domain.KeySize || bytes.Equal(a, b) {
		t.Fatal("deriveV2Key must depend on the order of its inputs")
	}
}
