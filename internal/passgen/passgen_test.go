package passgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

const wantSHA = "8f5ca830b8bffb6fe39c9736c024a00a6a6411adb3f83a9be8bfeeb6e067ae69"

func TestEmbeddedHash(t *testing.T) {
	sum := sha256.Sum256(rawList)
	if got := hex.EncodeToString(sum[:]); got != wantSHA {
		t.Fatalf("wordlist sha256 = %s, want %s", got, wantSHA)
	}
}

func TestWordlistMatchesJS(t *testing.T) {
	src, err := os.ReadFile("../../web/js/wordlist.js")
	if err != nil {
		t.Fatal(err)
	}
	var js []string
	for _, m := range regexp.MustCompile(`(?m)^\s*'([a-z -]+)',?$`).FindAllStringSubmatch(string(src), -1) {
		js = append(js, strings.Fields(m[1])...)
	}
	words, err := Wordlist()
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != listSize || strings.Join(js, " ") != strings.Join(words, " ") {
		t.Fatalf("Go and JS wordlists differ (go %d, js %d)", len(words), len(js))
	}
	words[0] = "mutated"
	again, _ := Wordlist()
	if again[0] == "mutated" {
		t.Fatal("Wordlist returned shared slice")
	}
}

func TestParseList(t *testing.T) {
	cases := map[string][]byte{
		"empty":   nil,
		"short":   []byte("1111\tacid\n"),
		"no tab":  bytes.Repeat([]byte("1111 acid\n"), listSize),
		"no word": bytes.Repeat([]byte("1111\t\n"), listSize),
	}
	for name, in := range cases {
		if _, err := parseList(in); !errors.Is(err, ErrWordlist) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if w, err := parseList(bytes.Repeat([]byte("1111\tx\n"), listSize)); err != nil || len(w) != listSize {
		t.Fatalf("valid list: %v", err)
	}
}

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for range 20 {
		p, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if len(p) < Words*3 || !unicode.IsUpper(rune(p[0])) {
			t.Fatalf("bad passphrase %q", p)
		}
		seen[p] = true
	}
	if len(seen) < 19 {
		t.Fatalf("passphrases repeat: %d unique", len(seen))
	}
}

func TestGenerateDeterministic(t *testing.T) {
	// 0xFFFF is >= limit and must be redrawn; 0x0000 picks "acid", 0x0001 "acorn".
	r := bytes.NewReader([]byte{0xff, 0xff, 0, 0, 0, 1, 0, 0, 0xff, 0xf0, 0, 1, 0x05, 0x10})
	p, err := generate(r)
	if err != nil {
		t.Fatal(err)
	}
	// 0x0510 = 1296 -> index 0.
	if p != "AcidAcornAcidAcornAcid" {
		t.Fatalf("got %q", p)
	}
}

func TestGenerateRandomFailure(t *testing.T) {
	if _, err := generate(bytes.NewReader([]byte{1})); err == nil {
		t.Fatal("expected error from short random source")
	}
}

func TestGenerateWordlistFailure(t *testing.T) {
	orig := wordlist
	t.Cleanup(func() { wordlist = orig })
	wordlist = func() ([]string, error) { return nil, ErrWordlist }
	if _, err := Generate(); !errors.Is(err, ErrWordlist) {
		t.Fatalf("Generate err = %v", err)
	}
	if _, err := Wordlist(); !errors.Is(err, ErrWordlist) {
		t.Fatalf("Wordlist err = %v", err)
	}
}
