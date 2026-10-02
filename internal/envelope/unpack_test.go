package envelope

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestUnpackLenient(t *testing.T) {
	cases := []struct {
		name   string
		header string
		body   string
		msg    string
		files  []File
	}{
		{"whitespace", " {\"v\" : 2 ,\n\"msg\":1,\"files\":[ ]} ", "a", "a", nil},
		{"key order", `{"files":[],"msg":1,"v":2}`, "a", "a", nil},
		{"unknown keys", `{"v":2,"msg":1,"files":[{"name":"n","type":"text/csv","size":0,"x":1}],"extra":{}}`, "a", "a", []File{{Name: "n", Type: "text/csv", Data: []byte{}}}},
		{"duplicate last wins", `{"v":1,"v":2,"msg":9,"msg":1,"files":[]}`, "a", "a", nil},
		{"number forms", `{"v":2.0,"msg":1e0,"files":[{"name":"n","type":"t","size":-0},{"name":"m","type":"t","size":1e-4000},{"name":"o","type":"t","size":-1e-4000}]}`, "a", "a",
			[]File{{Name: "n", Type: DefaultType, Data: []byte{}}, {Name: "m", Type: DefaultType, Data: []byte{}}, {Name: "o", Type: DefaultType, Data: []byte{}}}},
		{"resanitize", `{"v":2,"msg":0,"files":[{"name":"../\u202eevil","type":"TEXT/HTML","size":1}]}`, "z", "", []File{{Name: "evil", Type: DefaultType, Data: []byte("z")}}},
		{"lone surrogates", `{"v":2,"msg":0,"files":[{"name":"\udc00\ud800","type":"","size":0}]}`, "", "", []File{{Name: "\ufffd\ufffd", Type: DefaultType, Data: []byte{}}}},
		{"escaped keys", `{"\u0076":2,"msg":0,"files":[]}`, "", "", nil},
	}
	for _, c := range cases {
		p, err := Unpack(raw(c.header, []byte(c.body)))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(p.Message) != c.msg || len(p.Files) != len(c.files) {
			t.Errorf("%s: %+v", c.name, p)
			continue
		}
		for i, f := range c.files {
			g := p.Files[i]
			if g.Name != f.Name || g.Type != f.Type || !bytes.Equal(g.Data, f.Data) {
				t.Errorf("%s/%d: %+v", c.name, i, g)
			}
		}
	}
}

func TestUnpackRejects(t *testing.T) {
	file := func(size string) string {
		return fmt.Sprintf(`{"v":2,"msg":0,"files":[{"name":"n","type":"t","size":%s}]}`, size)
	}
	elevenFiles := `{"v":2,"msg":0,"files":[` + strings.TrimSuffix(strings.Repeat(`{"name":"n","type":"t","size":0},`, 11), ",") + `]}`
	cases := []struct {
		name string
		b    []byte
		want error
	}{
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
		{"too many files", raw(elevenFiles, nil), ErrTooManyFiles},
		{"file null", raw(`{"v":2,"msg":0,"files":[null]}`, nil), ErrInvalidEnvelope},
		{"file array", raw(`{"v":2,"msg":0,"files":[[]]}`, nil), ErrInvalidEnvelope},
		{"name missing", raw(`{"v":2,"msg":0,"files":[{"type":"t","size":0}]}`, nil), ErrInvalidEnvelope},
		{"name number", raw(`{"v":2,"msg":0,"files":[{"name":1,"type":"t","size":0}]}`, nil), ErrInvalidEnvelope},
		{"type null", raw(`{"v":2,"msg":0,"files":[{"name":"n","type":null,"size":0}]}`, nil), ErrInvalidEnvelope},
		{"size string", raw(file(`"0"`), nil), ErrInvalidEnvelope},
		{"size negative", raw(file("-1"), []byte{}), ErrInvalidEnvelope},
		{"size fraction", raw(file("1.5"), []byte("ab")), ErrInvalidEnvelope},
		{"size max", raw(file("9007199254740991"), nil), ErrInvalidEnvelope},
		{"body short", raw(`{"v":2,"msg":2,"files":[]}`, []byte("a")), ErrInvalidEnvelope},
		{"body long", raw(`{"v":2,"msg":0,"files":[]}`, []byte("a")), ErrInvalidEnvelope},
		{"file short", raw(file("2"), []byte("a")), ErrInvalidEnvelope},
		{"sum overflow", raw(`{"v":2,"msg":9007199254740991,"files":[{"name":"n","type":"t","size":9007199254740991}]}`, nil), ErrInvalidEnvelope},
	}
	for _, c := range cases {
		p, err := Unpack(c.b)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
		if p.Message != nil || p.Files != nil {
			t.Errorf("%s: partial payload returned", c.name)
		}
	}
}

func TestSizesMatch(t *testing.T) {
	max := uint64(1<<64 - 1)
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
		{1, []uint64{max}, max, false},
		{max, []uint64{1}, max, false},
	}
	for i, c := range cases {
		hs := make([]fileHeader, len(c.sizes))
		for j, s := range c.sizes {
			hs[j].size = s
		}
		if got := sizesMatch(c.msg, hs, c.avail); got != c.want {
			t.Errorf("%d: got %v", i, got)
		}
	}
}
