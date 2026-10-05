package envelope

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestNewKey(t *testing.T) {
	k1, err := NewKey()
	if err != nil || len(k1) != domain.KeySize {
		t.Fatalf("NewKey = %d bytes, %v", len(k1), err)
	}
	k2, _ := NewKey()
	if bytes.Equal(k1, k2) {
		t.Fatal("two keys are equal")
	}
	if _, err := newKeyFrom(errReader{}); err == nil {
		t.Fatal("expected RNG error")
	}
}

func TestSealOpen(t *testing.T) {
	key, _ := NewKey()
	pt := []byte("hello")
	nonce, ct, err := Seal(key, pt)
	if err != nil || len(nonce) != domain.NonceSize || len(ct) != len(pt)+domain.TagSize {
		t.Fatalf("Seal: %v nonce=%d ct=%d", err, len(nonce), len(ct))
	}
	got, err := Open(key, nonce, ct)
	if err != nil || !bytes.Equal(got, pt) {
		t.Fatalf("Open = %q, %v", got, err)
	}
	n2, _, _ := Seal(key, pt)
	if bytes.Equal(nonce, n2) {
		t.Fatal("nonce reused")
	}
}

func TestSealErrors(t *testing.T) {
	if _, _, err := Seal(make([]byte, 16), nil); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("AES-128 key: %v", err)
	}
	if _, _, err := sealFrom(errReader{}, make([]byte, domain.KeySize), nil); err == nil {
		t.Fatal("expected RNG error")
	}
}

func TestOpenGenericError(t *testing.T) {
	key, _ := NewKey()
	nonce, ct, _ := Seal(key, []byte("secret"))
	flip := func(b []byte, i int) []byte { c := bytes.Clone(b); c[i] ^= 1; return c }
	other, _ := NewKey()
	cases := []struct {
		name           string
		key, nonce, ct []byte
	}{
		{"wrong key", other, nonce, ct},
		{"short key", key[:16], nonce, ct},
		{"long key", append(bytes.Clone(key), 0), nonce, ct},
		{"short nonce", key, nonce[:11], ct},
		{"long nonce", key, append(bytes.Clone(nonce), 0), ct},
		{"flipped nonce", key, flip(nonce, 0), ct},
		{"flipped body", key, nonce, flip(ct, 0)},
		{"flipped tag", key, nonce, flip(ct, len(ct)-1)},
		{"truncated", key, nonce, ct[:len(ct)-1]},
		{"shorter than tag", key, nonce, ct[:domain.TagSize-1]},
		{"empty", key, nonce, nil},
	}
	for _, c := range cases {
		if pt, err := Open(c.key, c.nonce, c.ct); !errors.Is(err, ErrDecrypt) || pt != nil {
			t.Errorf("%s: got %q, %v", c.name, pt, err)
		}
	}
}
