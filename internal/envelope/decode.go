package envelope

import (
	"encoding/json"
	"unicode/utf8"
)

// decodeHeader parses GONE2 header JSON with the same semantics as
// JSON.parse plus the checks in docs/protocol.md §5.3: case-sensitive keys,
// last duplicate wins, unknown keys ignored, fatal UTF-8.
//
// Parameters:
//   - hb: header bytes.
//
// Returns the message size and sanitized file headers, or an error.
func decodeHeader(hb []byte) (uint64, []fileHeader, error) {
	var top map[string]json.RawMessage
	if !utf8.Valid(hb) || json.Unmarshal(hb, &top) != nil || top == nil {
		return 0, nil, invalid("header json")
	}
	if v, ok := jsonNumber(top["v"]); !ok || v != 2 {
		return 0, nil, invalid("version")
	}
	msg, ok := jsonSize(top["msg"])
	if !ok {
		return 0, nil, invalid("msg size")
	}
	hdrs, err := decodeFiles(top["files"])
	return msg, hdrs, err
}

// decodeFiles parses the "files" array.
//
// Parameters:
//   - raw: the raw "files" value.
//
// Returns sanitized file headers, ErrTooManyFiles, or an invalid-envelope
// error.
func decodeFiles(raw json.RawMessage) ([]fileHeader, error) {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil || arr == nil {
		return nil, invalid("files")
	}
	if len(arr) > MaxFiles {
		return nil, ErrTooManyFiles
	}
	hdrs := make([]fileHeader, len(arr))
	for i, el := range arr {
		h, ok := decodeFile(el)
		if !ok {
			return nil, invalid("file entry")
		}
		hdrs[i] = h
	}
	return hdrs, nil
}

// decodeFile parses one "files" element.
//
// Parameters:
//   - raw: the raw element.
//
// Returns the sanitized header and whether it was valid.
func decodeFile(raw json.RawMessage) (fileHeader, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return fileHeader{}, false
	}
	name, okN := jsonString(obj["name"])
	typ, okT := jsonString(obj["type"])
	size, okS := jsonSize(obj["size"])
	if !okN || !okT || !okS {
		return fileHeader{}, false
	}
	return fileHeader{name: SanitizeFileName(name), typ: SafeType(typ), size: size}, true
}
