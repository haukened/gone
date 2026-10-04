package envelope

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// checkSanitizedNameBasics verifies idempotence and basic filename safety.
//
// Parameters:
//   - t: the test handle.
//   - in: the original file name.
//   - out: the sanitized file name.
func checkSanitizedNameBasics(t *testing.T, in string, out string) {
	t.Helper()
	checkSanitizedNameIdempotent(t, in, out)
	checkSanitizedNameSafePath(t, out)
	checkSanitizedNameUTF8(t, out)
}

// checkSanitizedNameIdempotent verifies a sanitized name is stable.
//
// Parameters:
//   - t: the test handle.
//   - in: the original file name.
//   - out: the sanitized file name.
func checkSanitizedNameIdempotent(t *testing.T, in string, out string) {
	t.Helper()
	if SanitizeFileName(out) != out {
		t.Fatalf("not idempotent: %q -> %q", in, out)
	}
}

// checkSanitizedNameSafePath verifies a sanitized name is not path-like.
//
// Parameters:
//   - t: the test handle.
//   - out: the sanitized file name.
func checkSanitizedNameSafePath(t *testing.T, out string) {
	t.Helper()
	for _, bad := range []string{"", ".", ".."} {
		if out == bad {
			t.Fatalf("unsafe output %q", out)
		}
	}
	if strings.ContainsAny(out, `/\`) {
		t.Fatalf("unsafe output %q", out)
	}
}

// checkSanitizedNameUTF8 verifies a sanitized name is valid UTF-8 and bounded.
//
// Parameters:
//   - t: the test handle.
//   - out: the sanitized file name.
func checkSanitizedNameUTF8(t *testing.T, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Fatalf("invalid output %q", out)
	}
	if utf8.RuneCountInString(out) > maxNameRunes {
		t.Fatalf("invalid output %q", out)
	}
}

// checkSanitizedNameRunes verifies no banned code point survives sanitization.
//
// Parameters:
//   - t: the test handle.
//   - out: the sanitized file name.
func checkSanitizedNameRunes(t *testing.T, out string) {
	t.Helper()
	for _, r := range out {
		if inRanges(r, unsafeRanges) {
			t.Fatalf("banned code point %U in %q", r, out)
		}
	}
}
