package envelope

import (
	"strings"
	"unicode/utf8"
)

// DefaultType is used for any MIME type not on the allowlist.
const DefaultType = "application/octet-stream"

// safeTypes is the MIME allowlist shared with web/js/fileMeta.js.
var safeTypes = map[string]bool{
	"application/pdf": true, "application/zip": true, "application/gzip": true,
	"application/json": true, "application/x-tar": true, "application/x-7z-compressed": true,
	"text/plain": true, "text/csv": true,
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"audio/mpeg": true, "video/mp4": true,
}

// SafeType maps an untrusted MIME type onto the allowlist
// (docs/protocol.md §6.2).
//
// Parameters:
//   - t: untrusted MIME type, possibly with parameters.
//
// Returns an allowlisted, lowercase type or DefaultType.
func SafeType(t string) string {
	base, _, _ := strings.Cut(t, ";")
	base = strings.TrimFunc(base, isJSSpace)
	if !isASCII(base) {
		return DefaultType
	}
	base = strings.ToLower(base)
	if safeTypes[base] {
		return base
	}
	return DefaultType
}

// isASCII reports whether s contains only ASCII bytes.
//
// Parameters:
//   - s: string to check.
//
// Returns true when every byte is below utf8.RuneSelf.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
