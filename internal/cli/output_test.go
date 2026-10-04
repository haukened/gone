package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestWritersReportErrors checks write failures become I/O errors.
//
// Parameters:
//   - t: the test handle.
func TestWritersReportErrors(t *testing.T) {
	checks := map[string]error{
		"json":  writeJSON(errWriter{}, map[string]string{"a": "b"}),
		"text":  writeText(errWriter{}, "x"),
		"bytes": writeBytes(errWriter{}, []byte("x")),
	}
	for name, err := range checks {
		if classify(err).code != exitIO {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if err := writeJSON(errWriter{}, func() {}); err == nil {
		t.Error("unencodable value accepted")
	}
}

// TestWriteJSONNoHTMLEscape checks links are not HTML-escaped.
//
// Parameters:
//   - t: the test handle.
func TestWriteJSONNoHTMLEscape(t *testing.T) {
	var b strings.Builder
	if err := writeJSON(&b, map[string]string{"l": "a&b<c>"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "a&b<c>") {
		t.Fatalf("got %q", b.String())
	}
}

// TestWriteJSONEscapesControls checks DEL, C1 and bidi controls are written
// as \u escapes that decode back to the original string, while ordinary
// non-ASCII text stays raw.
//
// Parameters:
//   - t: the test handle.
func TestWriteJSONEscapesControls(t *testing.T) {
	in := "a\x7fb\u009b[31mc\u202ed\u2066é日"
	var b strings.Builder
	if err := writeJSON(&b, map[string]string{"m": in}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	requireJSONControlEscapes(t, out)
	requireJSONRoundTrip(t, out, in)
}

// requireJSONControlEscapes verifies escaped controls and raw safe Unicode.
//
// Parameters:
//   - t: the test handle.
//   - out: encoded JSON output.
func requireJSONControlEscapes(t *testing.T, out string) {
	t.Helper()
	for _, raw := range []string{"\x7f", "\u009b", "\u202e", "\u2066"} {
		if strings.Contains(out, raw) {
			t.Errorf("raw %q in %q", raw, out)
		}
	}
	for _, esc := range []string{`\u007f`, `\u009b`, `\u202e`, `\u2066`, "é日"} {
		if !strings.Contains(out, esc) {
			t.Errorf("missing %q in %q", esc, out)
		}
	}
}

// requireJSONRoundTrip verifies JSON output decodes to the original value.
//
// Parameters:
//   - t: the test handle.
//   - out: encoded JSON output.
//   - want: original message value.
func requireJSONRoundTrip(t *testing.T, out string, want string) {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got["m"] != want {
		t.Fatalf("round trip = %q, want %q", got["m"], want)
	}
}

// TestEscapeTerminal checks control, bidi and invalid bytes are escaped.
//
// Parameters:
//   - t: the test handle.
func TestEscapeTerminal(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain text\n\tok", "plain text\n\tok"},
		{"crlf\r\nok", "crlf\r\nok"},
		{"cr\roverwrite", `cr\u000doverwrite`},
		{"\x1b[31mred", `\u001b[31mred`},
		{"del\x7f", `del\u007f`},
		{"c1\u009b", `c1\u009b`},
		{"bad\xffbyte", `bad\xffbyte`},
		{"bidi\u202etxt", `bidi\u202etxt`},
		{"iso\u2066x\u2069", `iso\u2066x\u2069`},
		{"emoji 🎈 é", "emoji 🎈 é"},
	}
	for _, tt := range tests {
		if got := escapeTerminal(tt.in); got != tt.want {
			t.Errorf("escapeTerminal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// FuzzEscapeTerminal checks the output is valid UTF-8 with no unsafe
// runes.
//
// Parameters:
//   - f: the fuzz handle.
func FuzzEscapeTerminal(f *testing.F) {
	for _, s := range []string{"", "a\x1b[0m", "\r", "\xff\xfe", "\u202e"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := escapeTerminal(s)
		if !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8: %q", out)
		}
		for i, r := range out {
			if r == '\r' && !strings.HasPrefix(out[i+1:], "\n") {
				t.Fatalf("bare CR in %q", out)
			}
			if r != '\r' && unsafeTerminalRune(r) {
				t.Fatalf("unsafe rune %U in %q", r, out)
			}
		}
	})
}
