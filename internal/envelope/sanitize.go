package envelope

import "strings"

const (
	// maxNameRunes caps sanitized file names, in code points.
	maxNameRunes = 255
	// fallbackName replaces empty, "." and ".." names.
	fallbackName = "file"
)

// unsafeRanges lists code points stripped from file names
// (docs/protocol.md §6): controls, bidi and zero-width formatting.
var unsafeRanges = [][2]rune{
	{0x0000, 0x001f}, {0x007f, 0x009f}, {0x061c, 0x061c}, {0x180e, 0x180e},
	{0x200b, 0x200f}, {0x202a, 0x202e}, {0x2060, 0x206f}, {0xfeff, 0xfeff},
}

// jsSpaceRanges is the code point set trimmed by ECMAScript
// String.prototype.trim.
var jsSpaceRanges = [][2]rune{
	{0x0009, 0x000d}, {0x0020, 0x0020}, {0x00a0, 0x00a0}, {0x1680, 0x1680},
	{0x2000, 0x200a}, {0x2028, 0x2029}, {0x202f, 0x202f}, {0x205f, 0x205f},
	{0x3000, 0x3000}, {0xfeff, 0xfeff},
}

// inRanges reports whether r falls in any inclusive range.
//
// Parameters:
//   - r: code point.
//   - ranges: inclusive [lo, hi] pairs.
//
// Returns true when r is in a range.
func inRanges(r rune, ranges [][2]rune) bool {
	for _, rg := range ranges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// isJSSpace reports whether r is trimmed by JavaScript's String.trim.
//
// Parameters:
//   - r: code point.
//
// Returns true for JS whitespace.
func isJSSpace(r rune) bool { return inRanges(r, jsSpaceRanges) }

// SanitizeFileName reduces an untrusted name to a single safe path segment
// (docs/protocol.md §6.1). Invalid UTF-8 is treated as U+FFFD. The result is
// never empty, ".", or "..", and contains no '/', '\\' or unsafe code point.
//
// Parameters:
//   - name: untrusted file name.
//
// Returns the sanitized name.
func SanitizeFileName(name string) string {
	out := stripUnsafeNameRunes(lastFileNameSegment(name))
	return fallbackSanitizedName(out)
}

// lastFileNameSegment returns the last non-empty slash- or backslash-delimited
// segment using the same splitting rules as JavaScript link handling.
//
// Parameters:
//   - name: untrusted file name.
//
// Returns the last segment, or "" when none exists.
func lastFileNameSegment(name string) string {
	segs := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}

// stripUnsafeNameRunes removes unsafe code points and enforces maxNameRunes.
//
// Parameters:
//   - segment: single untrusted path segment.
//
// Returns the trimmed safe-code-point portion of segment.
func stripUnsafeNameRunes(segment string) string {
	var b strings.Builder
	n := 0
	for _, r := range segment {
		if n == maxNameRunes {
			break
		}
		if !inRanges(r, unsafeRanges) {
			b.WriteRune(r)
			n++
		}
	}
	return strings.TrimFunc(b.String(), isJSSpace)
}

// fallbackSanitizedName replaces empty and dot-only sanitized names.
//
// Parameters:
//   - name: sanitized candidate.
//
// Returns name or fallbackName when name cannot safely identify a file.
func fallbackSanitizedName(name string) string {
	if name == "" || name == "." || name == ".." {
		return fallbackName
	}
	return name
}
