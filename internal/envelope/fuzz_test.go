package envelope

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/haukened/gone/internal/domain"
)

func FuzzUnpack(f *testing.F) {
	for _, in := range unpackInputs() {
		f.Add(in.b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := Unpack(b)
		if err != nil {
			if !errors.Is(err, ErrInvalidEnvelope) && !errors.Is(err, ErrTooManyFiles) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		checkUnpacked(t, b, p)
	})
}

// checkUnpacked asserts that a successfully decoded payload is consistent
// with its input and survives a Pack/Unpack round trip.
func checkUnpacked(t *testing.T, b []byte, p Payload) {
	if len(p.Files) > MaxFiles {
		t.Fatal("too many files accepted")
	}
	n := len(p.Message)
	for _, f := range p.Files {
		n += len(f.Data)
		if SanitizeFileName(f.Name) != f.Name || SafeType(f.Type) != f.Type {
			t.Fatalf("unsanitized metadata %q %q", f.Name, f.Type)
		}
	}
	if !bytes.HasPrefix(b, Magic[:]) {
		return
	}
	if n > len(b) {
		t.Fatal("payload larger than input")
	}
	again, err := Pack(p)
	if err != nil {
		t.Fatalf("repack: %v", err)
	}
	q, err := Unpack(again)
	if err != nil || !bytes.Equal(q.Message, p.Message) || len(q.Files) != len(p.Files) {
		t.Fatalf("round trip mismatch: %v", err)
	}
}

func FuzzParseFragment(f *testing.F) {
	for _, s := range fragmentInputs() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fr, err := ParseFragment(s)
		if err != nil {
			if !errors.Is(err, ErrInvalidFragment) && !errors.Is(err, domain.ErrInvalidVersion) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if fr.String() != s || len(fr.Key()) != domain.KeySize {
			t.Fatalf("accepted non-canonical fragment %q", s)
		}
	})
}

func FuzzSanitizeFileName(f *testing.F) {
	for _, s := range buildSanitize().Names {
		f.Add(s.In)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := SanitizeFileName(s)
		if SanitizeFileName(out) != out {
			t.Fatalf("not idempotent: %q -> %q", s, out)
		}
		if out == "" || out == "." || out == ".." || strings.ContainsAny(out, `/\`) {
			t.Fatalf("unsafe output %q", out)
		}
		if !utf8.ValidString(out) || utf8.RuneCountInString(out) > maxNameRunes {
			t.Fatalf("invalid output %q", out)
		}
		for _, r := range out {
			if inRanges(r, unsafeRanges) {
				t.Fatalf("banned code point %U in %q", r, out)
			}
		}
	})
}
