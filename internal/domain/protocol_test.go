package domain

import (
	"errors"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want uint8
		err  error
	}{
		{"1", 1, nil},
		{"2", 2, nil},
		{"", 0, ErrInvalidVersion},
		{"0", 0, ErrInvalidVersion},
		{"01", 0, ErrInvalidVersion},
		{"+1", 0, ErrInvalidVersion},
		{"-1", 0, ErrInvalidVersion},
		{" 1", 0, ErrInvalidVersion},
		{"1 ", 0, ErrInvalidVersion},
		{"3", 0, ErrInvalidVersion},
		{"02", 0, ErrInvalidVersion},
		{"255", 0, ErrInvalidVersion},
		{"256", 0, ErrInvalidVersion},
		{"1000", 0, ErrInvalidVersion},
		{"1a", 0, ErrInvalidVersion},
		{"\u0661", 0, ErrInvalidVersion},
	}
	for _, c := range cases {
		got, err := ParseVersion(c.in)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("ParseVersion(%q) = %d, %v; want %d, %v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestSupported(t *testing.T) {
	for v := 0; v <= 255; v++ {
		if got := Supported(uint8(v)); got != (v == 1 || v == 2) { // #nosec G115 -- bounded loop
			t.Fatalf("Supported(%d) = %v", v, got)
		}
	}
}

func TestValidateProtocol(t *testing.T) {
	good := EncodeB64URL(make([]byte, NonceSize))
	cases := []struct {
		name  string
		v     uint8
		nonce string
		err   error
	}{
		{"ok", 1, good, nil},
		{"ok v2", 2, good, nil},
		{"bad version", 3, good, ErrInvalidVersion},
		{"short nonce v2", 2, EncodeB64URL(make([]byte, 11)), ErrInvalidNonce},
		{"zero version", 0, good, ErrInvalidVersion},
		{"empty nonce", 1, "", ErrInvalidNonce},
		{"short nonce", 1, EncodeB64URL(make([]byte, 11)), ErrInvalidNonce},
		{"long nonce", 1, EncodeB64URL(make([]byte, 13)), ErrInvalidNonce},
		{"padded", 1, good + "=", ErrInvalidNonce},
		{"std alphabet", 1, "AAAAAAAAAAAAAA+/", ErrInvalidNonce},
	}
	for _, c := range cases {
		if err := ValidateProtocol(c.v, c.nonce); !errors.Is(err, c.err) {
			t.Errorf("%s: got %v want %v", c.name, err, c.err)
		}
	}
}
