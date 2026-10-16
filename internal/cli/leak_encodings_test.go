package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// minNeedle is the shortest encoded form worth searching for; anything
// shorter could match by chance.
const minNeedle = 12

// jsonEscapedAll writes every byte of s as a \u00XX escape, the longest
// way a JSON encoder could legally spell it.
//
// Parameters:
//   - s: secret value.
//
// Returns the escaped text.
func jsonEscapedAll(s string) string {
	var b strings.Builder
	for i := range len(s) {
		fmt.Fprintf(&b, `\u%04x`, s[i])
	}
	return b.String()
}

// jsonEscaped returns s as encoding/json spells it inside a string, with
// HTML escaping on (the stricter spelling).
//
// Parameters:
//   - s: secret value.
//
// Returns the escaped text without quotes.
func jsonEscaped(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// base64Cores returns the base64 text of s that stays the same wherever s
// sits in a longer byte stream: s is encoded at each of the three byte
// alignments and the characters that depend on neighbouring bytes (and the
// padding) are dropped.
//
// Parameters:
//   - s: secret value.
//   - enc: base64 alphabet, unpadded.
//
// Returns up to three needles.
func base64Cores(s string, enc *base64.Encoding) []string {
	var out []string
	for pad := range 3 {
		e := enc.EncodeToString([]byte(strings.Repeat("\x00", pad) + s))
		lo, hi := (pad*8+5)/6+1, len(e)-2
		if hi > lo {
			out = append(out, e[lo:hi])
		}
	}
	return out
}

// tokenDigests returns the SHA-256 hex digest the server stores for a token
// string, and of the bytes it decodes to when it is base64url.
//
// Parameters:
//   - s: secret value.
//
// Returns one or two digests.
func tokenDigests(s string) []string {
	sum := sha256.Sum256([]byte(s))
	out := []string{hex.EncodeToString(sum[:])}
	if raw, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		sum = sha256.Sum256(raw)
		out = append(out, hex.EncodeToString(sum[:]))
	}
	return out
}

// encodedForms lists the case-sensitive spellings of a secret that must not
// be printed besides the secret itself.
//
// Parameters:
//   - s: secret value.
//
// Returns name to needle.
func encodedForms(s string) map[string]string {
	forms := map[string]string{
		"hex":                 hex.EncodeToString([]byte(s)),
		"url-percent (query)": url.QueryEscape(s),
		"url-percent (path)":  url.PathEscape(s),
		"json":                jsonEscaped(s),
		"json \\u00XX":        jsonEscapedAll(s),
	}
	for i, d := range tokenDigests(s) {
		forms[fmt.Sprintf("sha256 hex #%d", i)] = d
	}
	for i, c := range base64Cores(s, base64.RawStdEncoding) {
		forms[fmt.Sprintf("base64 std #%d", i)] = c
	}
	for i, c := range base64Cores(s, base64.RawURLEncoding) {
		forms[fmt.Sprintf("base64url #%d", i)] = c
	}
	return forms
}

// encodedLeaks returns "secret name as form" for every secret that appears
// in out as hex, standard base64, base64url, URL-percent-encoded text or
// JSON-escaped text, or whose SHA-256 digest (what the server stores for a
// token) appears. Hex and percent forms are compared ignoring case, since
// both may legally be written in either case.
//
// Parameters:
//   - out: captured output.
//   - secrets: secret name to value.
//
// Returns the findings, sorted by map order.
func encodedLeaks(out string, secrets map[string]string) []string {
	var found []string
	lower := strings.ToLower(out)
	for what, secret := range secrets {
		for form, needle := range encodedForms(secret) {
			if len(needle) >= minNeedle && (strings.Contains(out, needle) || strings.Contains(lower, strings.ToLower(needle))) {
				found = append(found, what+" as "+form)
			}
		}
	}
	return found
}

// requireNoEncodedLeak fails if any recorded secret appears in stdout or
// stderr in an encoded form (see encodedLeaks).
//
// Parameters:
//   - t: the test.
//   - name: case name for the report.
//   - l: environment with captured output and secrets.
func requireNoEncodedLeak(t *testing.T, name string, l *leakEnv) {
	t.Helper()
	out := l.te.stdout.String() + l.te.stderr.String()
	for _, f := range encodedLeaks(out, l.secrets) {
		t.Errorf("%s: %s leaked in %q", name, f, out)
	}
}

// TestNoEncodedSecretInOutput runs every leak case and checks that no
// secret is printed in an encoded form either.
//
// Parameters:
//   - t: the test.
func TestNoEncodedSecretInOutput(t *testing.T) {
	forEachLeakCase(t, requireNoEncodedLeak)
}

// TestEncodedFormsDetectLeaks checks the detector itself: an output that
// embeds a secret in each spelling is reported, a clean one is not.
//
// Parameters:
//   - t: the test.
func TestEncodedFormsDetectLeaks(t *testing.T) {
	const secret = `tok"en<&>/+-ABCDEFGHIJKLMNOP`
	spellings := []string{
		hex.EncodeToString([]byte(secret)),
		strings.ToUpper(hex.EncodeToString([]byte(secret))),
		base64.StdEncoding.EncodeToString([]byte(secret)),
		"xy" + base64.RawURLEncoding.EncodeToString([]byte("a"+secret)) + "zz",
		"xy" + base64.RawStdEncoding.EncodeToString([]byte("ab"+secret)) + "zz",
		url.QueryEscape(secret),
		strings.ToLower(url.QueryEscape(secret)),
		jsonEscaped(secret),
		jsonEscapedAll(secret),
		tokenDigests(secret)[0],
	}
	secrets := map[string]string{"s": secret}
	for _, spelled := range spellings {
		if len(encodedLeaks("before "+spelled+" after", secrets)) == 0 {
			t.Errorf("not detected: %q", spelled)
		}
	}
	if got := encodedLeaks("nothing to see here, sha256 of something else", secrets); len(got) != 0 {
		t.Errorf("false positive: %v", got)
	}
	for _, d := range tokenDigests("QUJD") {
		if len(d) != 64 {
			t.Errorf("digest %q", d)
		}
	}
	if len(tokenDigests("QUJD")) != 2 || len(tokenDigests("not base64!")) != 1 {
		t.Error("digest count")
	}
}
