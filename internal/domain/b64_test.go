package domain

import (
	"bytes"
	"errors"
	"testing"
)

func TestB64URL(t *testing.T) {
	valid := []struct {
		in   string
		want []byte
	}{
		{"", []byte{}},
		{"AA", []byte{0}},
		{"AAA", []byte{0, 0}},
		{"AAAA", []byte{0, 0, 0}},
		{"-_8", []byte{0xfb, 0xff}},
	}
	for _, c := range valid {
		got, err := DecodeB64URL(c.in)
		if err != nil || !bytes.Equal(got, c.want) {
			t.Errorf("DecodeB64URL(%q) = %x, %v", c.in, got, err)
		}
		if enc := EncodeB64URL(c.want); enc != c.in {
			t.Errorf("EncodeB64URL(%x) = %q want %q", c.want, enc, c.in)
		}
	}
	invalid := []string{
		"A",        // len%4 == 1
		"AB",       // non-canonical trailing bits
		"AAB",      // non-canonical trailing bits
		"AA==",     // padding
		"AA\nAA",   // newline (ignored by encoding/base64)
		"AA\rAA",   // carriage return
		"AA+A",     // standard alphabet
		"AA/A",     // standard alphabet
		"AA A",     // space
		"AA%41",    // percent escape
		"AAAA\x00", // NUL
	}
	for _, in := range invalid {
		if _, err := DecodeB64URL(in); !errors.Is(err, ErrInvalidB64) {
			t.Errorf("DecodeB64URL(%q) err = %v, want ErrInvalidB64", in, err)
		}
	}
}
