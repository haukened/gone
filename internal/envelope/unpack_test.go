package envelope

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type unpackLenientCase struct {
	name   string
	header string
	body   string
	msg    string
	files  []File
}

type unpackRejectCase struct {
	name string
	b    []byte
	want error
}

var unpackLenientCases = []unpackLenientCase{
	{"whitespace", " {\"v\" : 2 ,\n\"msg\":1,\"files\":[ ]} ", "a", "a", nil},
	{"key order", `{"files":[],"msg":1,"v":2}`, "a", "a", nil},
	{"unknown keys", `{"v":2,"msg":1,"files":[{"name":"n","type":"text/csv","size":0,"x":1}],"extra":{}}`, "a", "a", []File{{Name: "n", Type: "text/csv", Data: []byte{}}}},
	{"duplicate last wins", `{"v":1,"v":2,"msg":9,"msg":1,"files":[]}`, "a", "a", nil},
	{"number forms", `{"v":2.0,"msg":1e0,"files":[{"name":"n","type":"t","size":-0},{"name":"m","type":"t","size":1e-4000},{"name":"o","type":"t","size":-1e-4000}]}`, "a", "a", []File{{Name: "n", Type: DefaultType, Data: []byte{}}, {Name: "m", Type: DefaultType, Data: []byte{}}, {Name: "o", Type: DefaultType, Data: []byte{}}}},
	{"resanitize", `{"v":2,"msg":0,"files":[{"name":"../\u202eevil","type":"TEXT/HTML","size":1}]}`, "z", "", []File{{Name: "evil", Type: DefaultType, Data: []byte("z")}}},
	{"lone surrogates", `{"v":2,"msg":0,"files":[{"name":"\udc00\ud800","type":"","size":0}]}`, "", "", []File{{Name: "\ufffd\ufffd", Type: DefaultType, Data: []byte{}}}},
	{"escaped keys", `{"\u0076":2,"msg":0,"files":[]}`, "", "", nil},
}

var unpackRejectCases = []unpackRejectCase{
	{"truncated prefix", Magic[:], ErrInvalidEnvelope},
	{"truncated len", append(Magic[:], 0, 0, 0), ErrInvalidEnvelope},
	{"header past end", append(append(Magic[:], 0, 0, 0, 9), "{}"...), ErrInvalidEnvelope},
	{"header over cap", append(append(Magic[:], 0, 0, 0x40, 1), make([]byte, MaxHeaderBytes+1)...), ErrInvalidEnvelope},
	{"huge len", append(Magic[:], 0xff, 0xff, 0xff, 0xff), ErrInvalidEnvelope},
	{"empty header", raw("", nil), ErrInvalidEnvelope},
	{"bad json", raw("{", nil), ErrInvalidEnvelope},
	{"null", raw("null", nil), ErrInvalidEnvelope},
	{"array", raw("[]", nil), ErrInvalidEnvelope},
	{"trailing", raw(`{"v":2,"msg":0,"files":[]}x`, nil), ErrInvalidEnvelope},
	{"bom", raw("\ufeff"+`{"v":2,"msg":0,"files":[]}`, nil), ErrInvalidEnvelope},
	{"bad utf8", raw(`{"v":2,"msg":0,"files":[],"x":"`+"\xff"+`"}`, nil), ErrInvalidEnvelope},
	{"no v", raw(`{"msg":0,"files":[]}`, nil), ErrInvalidEnvelope},
	{"v1", raw(`{"v":1,"msg":0,"files":[]}`, nil), ErrInvalidEnvelope},
	{"v string", raw(`{"v":"2","msg":0,"files":[]}`, nil), ErrInvalidEnvelope},
	{"V case", raw(`{"V":2,"msg":0,"files":[]}`, nil), ErrInvalidEnvelope},
	{"no msg", raw(`{"v":2,"files":[]}`, nil), ErrInvalidEnvelope},
	{"msg string", raw(`{"v":2,"msg":"0","files":[]}`, nil), ErrInvalidEnvelope},
	{"msg null", raw(`{"v":2,"msg":null,"files":[]}`, nil), ErrInvalidEnvelope},
	{"msg negative", raw(`{"v":2,"msg":-1,"files":[]}`, nil), ErrInvalidEnvelope},
	{"msg fraction", raw(`{"v":2,"msg":0.5,"files":[]}`, nil), ErrInvalidEnvelope},
	{"msg overflow", raw(`{"v":2,"msg":1e400,"files":[]}`, nil), ErrInvalidEnvelope},
	{"msg 2^53", raw(`{"v":2,"msg":9007199254740992,"files":[]}`, nil), ErrInvalidEnvelope},
	{"no files", raw(`{"v":2,"msg":0}`, nil), ErrInvalidEnvelope},
	{"files null", raw(`{"v":2,"msg":0,"files":null}`, nil), ErrInvalidEnvelope},
	{"files object", raw(`{"v":2,"msg":0,"files":{}}`, nil), ErrInvalidEnvelope},
	{"too many files", raw(elevenFileHeader(), nil), ErrTooManyFiles},
	{"file null", raw(`{"v":2,"msg":0,"files":[null]}`, nil), ErrInvalidEnvelope},
	{"file array", raw(`{"v":2,"msg":0,"files":[[]]}`, nil), ErrInvalidEnvelope},
	{"name missing", raw(`{"v":2,"msg":0,"files":[{"type":"t","size":0}]}`, nil), ErrInvalidEnvelope},
	{"name number", raw(`{"v":2,"msg":0,"files":[{"name":1,"type":"t","size":0}]}`, nil), ErrInvalidEnvelope},
	{"type null", raw(`{"v":2,"msg":0,"files":[{"name":"n","type":null,"size":0}]}`, nil), ErrInvalidEnvelope},
	{"size string", raw(rejectSizeHeader(`"0"`), nil), ErrInvalidEnvelope},
	{"size negative", raw(rejectSizeHeader("-1"), []byte{}), ErrInvalidEnvelope},
	{"size fraction", raw(rejectSizeHeader("1.5"), []byte("ab")), ErrInvalidEnvelope},
	{"size max", raw(rejectSizeHeader("9007199254740991"), nil), ErrInvalidEnvelope},
	{"body short", raw(`{"v":2,"msg":2,"files":[]}`, []byte("a")), ErrInvalidEnvelope},
	{"body long", raw(`{"v":2,"msg":0,"files":[]}`, []byte("a")), ErrInvalidEnvelope},
	{"file short", raw(rejectSizeHeader("2"), []byte("a")), ErrInvalidEnvelope},
	{"sum overflow", raw(`{"v":2,"msg":9007199254740991,"files":[{"name":"n","type":"t","size":9007199254740991}]}`, nil), ErrInvalidEnvelope},
}

// rejectSizeHeader builds a one-file GONE2 header with the provided size value.
//
// Parameters:
//   - size: the JSON size value to insert.
//
// Returns:
//   - string: the encoded test header.
func rejectSizeHeader(size string) string {
	return fmt.Sprintf(`{"v":2,"msg":0,"files":[{"name":"n","type":"t","size":%s}]}`, size)
}

// elevenFileHeader builds a GONE2 header containing eleven files.
//
// Returns:
//   - string: the encoded oversized file-list header.
func elevenFileHeader() string {
	files := strings.TrimSuffix(strings.Repeat(`{"name":"n","type":"t","size":0},`, 11), ",")
	return `{"v":2,"msg":0,"files":[` + files + `]}`
}

// TestUnpackLenient verifies tolerated JSON forms still decode correctly.
//
// Parameters:
//   - t: the test handle.
func TestUnpackLenient(t *testing.T) {
	for _, c := range unpackLenientCases {
		checkUnpackLenientCase(t, c)
	}
}

// checkUnpackLenientCase verifies one lenient Unpack case.
//
// Parameters:
//   - t: the test handle.
//   - c: the lenient Unpack case to verify.
func checkUnpackLenientCase(t *testing.T, c unpackLenientCase) {
	t.Helper()
	p, err := Unpack(raw(c.header, []byte(c.body)))
	if err != nil {
		t.Errorf("%s: %v", c.name, err)
		return
	}
	if string(p.Message) != c.msg || len(p.Files) != len(c.files) {
		t.Errorf("%s: %+v", c.name, p)
		return
	}
	checkUnpackLenientFiles(t, c, p.Files)
}

// checkUnpackLenientFiles verifies decoded files for a lenient case.
//
// Parameters:
//   - t: the test handle.
//   - c: the lenient Unpack case to verify.
//   - got: the decoded files.
func checkUnpackLenientFiles(t *testing.T, c unpackLenientCase, got []File) {
	t.Helper()
	for i, f := range c.files {
		g := got[i]
		if g.Name != f.Name || g.Type != f.Type || !bytes.Equal(g.Data, f.Data) {
			t.Errorf("%s/%d: %+v", c.name, i, g)
		}
	}
}

// TestUnpackRejects verifies malformed envelopes are rejected cleanly.
//
// Parameters:
//   - t: the test handle.
func TestUnpackRejects(t *testing.T) {
	for _, c := range unpackRejectCases {
		checkUnpackRejectCase(t, c)
	}
}

// checkUnpackRejectCase verifies one rejecting Unpack case.
//
// Parameters:
//   - t: the test handle.
//   - c: the rejecting Unpack case to verify.
func checkUnpackRejectCase(t *testing.T, c unpackRejectCase) {
	t.Helper()
	p, err := Unpack(c.b)
	if !errors.Is(err, c.want) {
		t.Errorf("%s: %v", c.name, err)
	}
	if p.Message != nil || p.Files != nil {
		t.Errorf("%s: partial payload returned", c.name)
	}
}

// TestSizesMatch verifies payload size accounting, including overflow cases.
//
// Parameters:
//   - t: the test handle.
func TestSizesMatch(t *testing.T) {
	maxUint64 := uint64(1<<64 - 1)
	cases := []struct {
		msg   uint64
		sizes []uint64
		avail uint64
		want  bool
	}{
		{0, nil, 0, true},
		{1, []uint64{2, 3}, 6, true},
		{1, []uint64{2, 3}, 5, false},
		{1, []uint64{2, 3}, 7, false},
		{7, []uint64{0}, 6, false},
		{1, []uint64{maxUint64}, maxUint64, false},
		{maxUint64, []uint64{1}, maxUint64, false},
	}
	for i, c := range cases {
		checkSizesMatchCase(t, i, c.msg, c.sizes, c.avail, c.want)
	}
}

// checkSizesMatchCase verifies one sizesMatch table entry.
//
// Parameters:
//   - t: the test handle.
//   - i: the case index.
//   - msg: the message size.
//   - sizes: the file sizes.
//   - avail: the available body size.
//   - want: the expected sizesMatch result.
func checkSizesMatchCase(t *testing.T, i int, msg uint64, sizes []uint64, avail uint64, want bool) {
	t.Helper()
	hs := make([]fileHeader, len(sizes))
	for j, s := range sizes {
		hs[j].size = s
	}
	if got := sizesMatch(msg, hs, avail); got != want {
		t.Errorf("%d: got %v", i, got)
	}
}
