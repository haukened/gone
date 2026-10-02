package envelope

import (
	"encoding/json"
	"math"
	"strconv"
)

// shortEscapes are the two-character escapes JSON.stringify emits.
var shortEscapes = map[byte]string{
	'"': `\"`, '\\': `\\`, '\b': `\b`, '\t': `\t`, '\n': `\n`, '\f': `\f`, '\r': `\r`,
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON string, escaped exactly as ECMAScript
// JSON.stringify does for well-formed input (docs/protocol.md §5.4). encoding/json
// is not used because it also escapes <, >, &, U+2028 and U+2029.
//
// Parameters:
//   - dst: buffer to append to.
//   - s: valid UTF-8 string.
//
// Returns the extended buffer.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if esc, ok := shortEscapes[c]; ok {
			dst = append(dst, esc...)
			continue
		}
		if c < 0x20 {
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			continue
		}
		dst = append(dst, c)
	}
	return append(dst, '"')
}

// canonicalHeader renders the GONE2 header in canonical form:
// {"v":2,"msg":n,"files":[{"name":s,"type":s,"size":n},...]}.
//
// Parameters:
//   - msgLen: message length in bytes.
//   - files: already-sanitized file metadata.
//
// Returns the header bytes.
func canonicalHeader(msgLen int, files []fileHeader) []byte {
	out := []byte(`{"v":2,"msg":`)
	out = strconv.AppendInt(out, int64(msgLen), 10)
	out = append(out, `,"files":[`...)
	for i, f := range files {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, `{"name":`...)
		out = appendJSONString(out, f.name)
		out = append(out, `,"type":`...)
		out = appendJSONString(out, f.typ)
		out = append(out, `,"size":`...)
		out = strconv.AppendUint(out, f.size, 10)
		out = append(out, '}')
	}
	return append(out, "]}"...)
}

// jsonValue decodes raw into a generic JSON value (numbers as float64,
// matching JavaScript).
//
// Parameters:
//   - raw: raw JSON value; nil when the key was absent.
//
// Returns the value and whether decoding succeeded.
func jsonValue(raw json.RawMessage) (any, bool) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	return v, true
}

// jsonNumber decodes a JSON number as IEEE-754 binary64.
//
// Parameters:
//   - raw: raw JSON value.
//
// Returns the number and whether raw was a finite number.
func jsonNumber(raw json.RawMessage) (float64, bool) {
	v, _ := jsonValue(raw)
	f, ok := v.(float64)
	return f, ok
}

// jsonString decodes a JSON string.
//
// Parameters:
//   - raw: raw JSON value.
//
// Returns the string and whether raw was a string.
func jsonString(raw json.RawMessage) (string, bool) {
	v, _ := jsonValue(raw)
	s, ok := v.(string)
	return s, ok
}

// jsonSize decodes a size: a number whose binary64 value is an integer in
// [0, 2^53-1] (docs/protocol.md §5.2).
//
// Parameters:
//   - raw: raw JSON value.
//
// Returns the size and whether it was valid.
func jsonSize(raw json.RawMessage) (uint64, bool) {
	f, ok := jsonNumber(raw)
	if !ok || f < 0 || f > maxSafeInt || f != math.Trunc(f) {
		return 0, false
	}
	return uint64(f), true
}
