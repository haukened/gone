package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// writeJSON writes v as one line of JSON. Code points that are unsafe on a
// terminal but left raw by encoding/json (DEL, C1 controls, bidirectional
// controls) are written as \uXXXX escapes, so JSON output is as safe to
// print as text output.
//
// Parameters:
//   - w: destination.
//   - v: value to encode.
//
// Returns an encoding or I/O error, if any.
func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ioErr("write output", err)
	}
	return writeText(w, escapeJSONControls(buf.String()))
}

// escapeJSONControls rewrites terminal-unsafe code points in encoded JSON as
// \uXXXX escapes. encoding/json already escapes C0 controls and replaces
// invalid UTF-8, and these code points can only occur inside strings, so the
// result is equivalent JSON.
//
// Parameters:
//   - s: valid JSON produced by encoding/json.
//
// Returns the escaped JSON.
func escapeJSONControls(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x7f && unsafeTerminalRune(r) {
			fmt.Fprintf(&b, `\u%04x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// writeText writes s to w.
//
// Parameters:
//   - w: destination.
//   - s: text.
//
// Returns an I/O error, if any.
func writeText(w io.Writer, s string) error {
	if _, err := io.WriteString(w, s); err != nil {
		return ioErr("write output", err)
	}
	return nil
}

// writeBytes writes b to w.
//
// Parameters:
//   - w: destination.
//   - b: bytes.
//
// Returns an I/O error, if any.
func writeBytes(w io.Writer, b []byte) error {
	if _, err := w.Write(b); err != nil {
		return ioErr("write output", err)
	}
	return nil
}

// escapeTerminal makes untrusted text safe to print on a terminal by
// escaping C0 and C1 control characters, DEL, bidirectional and zero-width
// format characters (the set docs/protocol.md section 6 calls unsafe, except ZWNJ and ZWJ), a
// carriage return not followed by a line feed, and invalid UTF-8. Newlines
// and tabs are kept.
// This prevents a sender from injecting terminal escape sequences.
//
// Parameters:
//   - s: untrusted text.
//
// Returns the escaped text.
func escapeTerminal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\r' && strings.HasPrefix(s[i+1:], "\n"):
			b.WriteByte('\r')
		case unsafeTerminalRune(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// unsafeTerminalRune reports whether r must be escaped on a terminal.
//
// Parameters:
//   - r: code point.
//
// Returns true for control characters other than newline and tab, and for
// bidirectional and zero-width characters that can visually reorder or hide
// text.
func unsafeTerminalRune(r rune) bool {
	for _, rg := range unsafeTerminalRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// unsafeTerminalRanges lists inclusive code point ranges escaped on a
// terminal: C0 controls except tab and newline, DEL and C1 controls, and the
// invisible or reordering format characters of docs/protocol.md section 6:
// U+061C, U+180E, U+200B, U+200E-200F, U+202A-202E, U+2060-206F and U+FEFF.
// U+200C (ZWNJ) and U+200D (ZWJ) are deliberately not escaped: Persian, Arabic,
// Hindi and other scripts and emoji sequences need them to render correctly,
// and they can neither reorder nor hide text.
var unsafeTerminalRanges = [...][2]rune{
	{0x00, 0x08},
	{0x0b, 0x1f},
	{0x7f, 0x9f},
	{0x061c, 0x061c},
	{0x180e, 0x180e},
	{0x200b, 0x200b},
	{0x200e, 0x200f},
	{0x202a, 0x202e},
	{0x2060, 0x206f},
	{0xfeff, 0xfeff},
}
