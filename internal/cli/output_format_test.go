package cli

import (
	"bytes"
	"strings"
	"testing"
)

// isUnsafeCodePoint is an independent statement of the unsafe set from
// docs/protocol.md section 6 (controls other than tab and newline are
// covered by unsafeTerminalRune), so the fuzz test does not just reuse the
// table it checks.
func isUnsafeCodePoint(r rune) bool {
	switch {
	case r == 0x061c, r == 0x180e, r == 0xfeff:
		return true
	case r == 0x200b, r >= 0x200e && r <= 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2060 && r <= 0x206f:
		return true
	}
	return false
}

// TestJSONEscapesFormatCharacters checks JSON output also escapes the
// invisible and direction-changing code points.
func TestJSONEscapesFormatCharacters(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]string{"m": "a\u200eb\u200bc\ufeffd\u2060e\u061cf\u180eg"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, r := range out {
		if isUnsafeCodePoint(r) {
			t.Errorf("raw %U in %q", r, out)
		}
	}
	for _, esc := range []string{`\u200e`, `\u200b`, `\ufeff`, `\u2060`, `\u061c`, `\u180e`} {
		if !strings.Contains(out, esc) {
			t.Errorf("missing %s in %q", esc, out)
		}
	}
}
