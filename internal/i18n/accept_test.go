package i18n

import (
	"strings"
	"testing"
)

// TestSplitAccept_CapsEntries checks that a huge header yields at most
// maxAcceptEntries parts, regardless of its size.
func TestSplitAccept_CapsEntries(t *testing.T) {
	cases := map[string]string{
		"1MiB of short entries": strings.Repeat("en,", 1<<20/3),
		"1MiB without commas":   strings.Repeat("x", 1<<20),
		"1MiB of commas":        strings.Repeat(",", 1<<20),
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			if got := len(splitAccept(header)); got > maxAcceptEntries {
				t.Fatalf("splitAccept returned %d parts, want <= %d", got, maxAcceptEntries)
			}
		})
	}
	if got := len(splitAccept(strings.Repeat("en,", 1<<20/3))); got != maxAcceptEntries {
		t.Fatalf("splitAccept returned %d parts, want exactly %d", got, maxAcceptEntries)
	}
}

// TestSplitAccept_Small checks short headers split unchanged.
func TestSplitAccept_Small(t *testing.T) {
	got := splitAccept("fr;q=0.8,en")
	if len(got) != 2 || got[0] != "fr;q=0.8" || got[1] != "en" {
		t.Fatalf("got %q", got)
	}
	if got := splitAccept(""); len(got) != 0 {
		t.Fatalf("empty header gave %q", got)
	}
}

// TestParseAcceptLanguage_HugeHeaderBounded checks the parser considers only
// the first entries of a 1 MiB header and drops oversized ones.
func TestParseAcceptLanguage_HugeHeaderBounded(t *testing.T) {
	header := strings.Repeat("de;q=0.5,", 1<<20/9)
	if got := parseAcceptLanguage(header); len(got) != maxAcceptEntries {
		t.Fatalf("parsed %d entries, want %d", len(got), maxAcceptEntries)
	}
	// Entries after the cap are ignored even when they would win on quality.
	tail := strings.Repeat("de;q=0.1,", maxAcceptEntries) + "fr;q=1"
	for _, e := range parseAcceptLanguage(tail) {
		if e.tag == "fr" {
			t.Fatal("entry beyond the cap was parsed")
		}
	}
	for name, h := range map[string]string{
		"one 1MiB tag":    strings.Repeat("x", 1<<20),
		"1MiB parameters": "en" + strings.Repeat(";q=1", 1<<18),
		"1MiB semicolons": strings.Repeat(";", 1<<20),
	} {
		if got := parseAcceptLanguage(h); len(got) != 0 {
			t.Errorf("%s: parsed %d entries, want 0", name, len(got))
		}
	}
}

// TestParseAcceptLanguage_Order checks ordering and filtering still hold.
func TestParseAcceptLanguage_Order(t *testing.T) {
	got := parseAcceptLanguage("fr;q=0.5, en, *, de;q=0, es;q=bad, pt-BR;q=0.9")
	want := []string{"en", "pt-BR", "fr"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i, w := range want {
		if got[i].tag != w {
			t.Fatalf("entry %d = %q, want %q", i, got[i].tag, w)
		}
	}
}

// BenchmarkParseAcceptLanguage_1MiB measures parsing a 1 MiB header; the
// cost must not grow with the header size.
func BenchmarkParseAcceptLanguage_1MiB(b *testing.B) {
	header := strings.Repeat("en-US;q=0.8,", 1<<20/12)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseAcceptLanguage(header)
	}
}
