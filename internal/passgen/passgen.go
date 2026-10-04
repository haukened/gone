// Package passgen generates passphrases for the optional protocol v2
// passphrase. It mirrors web/js/passgen.js: five CamelCase words drawn with
// unbiased rejection sampling from the EFF Short Wordlist #1.
//
// The wordlist is copyright Electronic Frontier Foundation, licensed under
// CC BY 3.0 US (https://creativecommons.org/licenses/by/3.0/us/), and is
// embedded verbatim from
// https://www.eff.org/files/2016/09/08/eff_short_wordlist_1.txt
// (SHA-256 8f5ca830b8bffb6fe39c9736c024a00a6a6411adb3f83a9be8bfeeb6e067ae69).
package passgen

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Words is the number of words in a generated passphrase (about 51.7 bits).
const Words = 5

// listSize is the number of words in the EFF short wordlist #1.
const listSize = 1296

// sampleRange is the size of the 16-bit sample space used for each draw.
const sampleRange = 1 << 16

// limit is the largest multiple of listSize not above sampleRange; samples
// at or above it are redrawn so no word is favoured.
const limit = sampleRange - sampleRange%listSize

// ErrWordlist indicates the embedded wordlist is malformed.
var ErrWordlist = errors.New("passgen: malformed wordlist")

//go:embed eff_short_wordlist_1.txt
var rawList []byte

// wordlist parses the embedded list once.
var wordlist = sync.OnceValues(func() ([]string, error) { return parseList(rawList) })

// parseList parses the EFF "dddd\tword" format.
//
// Parameters:
//   - b: the raw wordlist file.
//
// Returns the words in file order, or ErrWordlist if the format or count is
// wrong.
func parseList(b []byte) ([]string, error) {
	lines := bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n"))
	if len(lines) != listSize {
		return nil, ErrWordlist
	}
	out := make([]string, 0, listSize)
	for _, ln := range lines {
		_, w, ok := bytes.Cut(ln, []byte("\t"))
		if !ok || len(w) == 0 {
			return nil, ErrWordlist
		}
		out = append(out, string(w))
	}
	return out, nil
}

// Wordlist returns a copy of the embedded EFF short wordlist #1.
//
// Returns the 1,296 words in source order, or ErrWordlist.
func Wordlist() ([]string, error) {
	w, err := wordlist()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), w...), nil
}

// Generate returns a new passphrase of Words CamelCase words, e.g.
// "FrostCanalBloomTrickRuby", using crypto/rand.
//
// Returns the passphrase, or an error if the wordlist or random source fails.
func Generate() (string, error) {
	return generate(rand.Reader)
}

// generate builds a passphrase using r as the randomness source.
//
// Parameters:
//   - r: source of uniformly random bytes.
//
// Returns the passphrase, or an error from the wordlist or r.
func generate(r io.Reader) (string, error) {
	list, err := wordlist()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for range Words {
		i, err := pickIndex(r, len(list))
		if err != nil {
			return "", err
		}
		w := list[i]
		sb.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return sb.String(), nil
}

// pickIndex returns a uniform index in [0, n) by rejection sampling 16-bit
// values from r.
//
// Parameters:
//   - r: source of random bytes.
//   - n: list length; must equal listSize so limit is correct.
//
// Returns the index, or an error if r fails.
func pickIndex(r io.Reader, n int) (int, error) {
	var buf [2]byte
	for {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, fmt.Errorf("passgen: random source: %w", err)
		}
		if v := int(binary.BigEndian.Uint16(buf[:])); v < limit {
			return v % n, nil
		}
	}
}
