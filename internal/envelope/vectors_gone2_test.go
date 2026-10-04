package envelope

import (
	"encoding/hex"
	"strings"
)

type fileVec struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data"`
}

type packCase struct {
	Name     string    `json:"name"`
	Message  string    `json:"message"`
	Files    []fileVec `json:"files"`
	Expected string    `json:"expected,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type unpackCase struct {
	Name    string    `json:"name"`
	Input   string    `json:"input"`
	Message string    `json:"message,omitempty"`
	Files   []fileVec `json:"files,omitempty"`
	Error   string    `json:"error,omitempty"`
}

type envelopeVectors struct {
	Description string       `json:"description"`
	Pack        []packCase   `json:"pack"`
	Unpack      []unpackCase `json:"unpack"`
}

// toFileVecs converts payload files into hex-encoded vector files.
//
// Parameters:
//   - fs: the payload files to convert.
//
// Returns:
//   - []fileVec: the vector files.
func toFileVecs(fs []File) []fileVec {
	out := make([]fileVec, len(fs))
	for i, f := range fs {
		out[i] = fileVec{Name: f.Name, Type: f.Type, Data: hx(f.Data)}
	}
	return out
}

// packInputs returns the source cases for Pack vectors.
//
// Returns:
//   - []packCase: the Pack vector source cases.
func packInputs() []packCase {
	tenFiles := make([]fileVec, MaxFiles)
	for i := range tenFiles {
		tenFiles[i] = fileVec{Name: "f", Type: "text/plain", Data: hx([]byte{byte(i)})}
	}
	return []packCase{
		{Name: "text only", Message: "hello"},
		{Name: "empty", Message: ""},
		{Name: "magic-prefixed text", Message: "GONE2\x00\x00\x00tail"},
		{Name: "one file", Message: "see attached", Files: []fileVec{{"report.pdf", "application/pdf", "255044462d"}}},
		{Name: "file only", Files: []fileVec{{"a.bin", "", ""}}},
		{Name: "sanitized metadata", Message: "x", Files: sanitizedPackFiles()},
		{Name: "ten files", Files: tenFiles},
		{Name: "eleven files", Files: append(append([]fileVec{}, tenFiles...), fileVec{"x", "", ""})},
	}
}

// sanitizedPackFiles returns file cases that exercise metadata sanitization.
//
// Returns:
//   - []fileVec: the metadata sanitization source files.
func sanitizedPackFiles() []fileVec {
	return []fileVec{
		{"../../etc/passwd", "Text/Plain; charset=utf-8", "00"},
		{"invoice\u202etxt.exe", "text/html", "01"},
		{"\u200b..\u200b", "IMAGE/PNG", ""},
		{"quote\"back\\slash\ttab", "image/svg+xml", "02"},
		{"日本😀.txt", "text/csv", "03"},
	}
}

// buildPack computes Pack vector outputs for each source case.
//
// Returns:
//   - []packCase: the completed Pack vector cases.
func buildPack() []packCase {
	cases := packInputs()
	for i, c := range cases {
		p := Payload{Message: []byte(c.Message)}
		for _, f := range c.Files {
			d, _ := hex.DecodeString(f.Data)
			p.Files = append(p.Files, File{Name: f.Name, Type: f.Type, Data: d})
		}
		out, err := Pack(p)
		if err != nil {
			cases[i].Error = errCode(err)
			continue
		}
		cases[i].Expected = hx(out)
	}
	return cases
}

// gone2Raw builds encoded GONE2 bytes from string components.
//
// Parameters:
//   - header: the JSON header string.
//   - body: the body string.
//
// Returns:
//   - []byte: the encoded GONE2 bytes.
func gone2Raw(header string, body string) []byte {
	return raw(header, []byte(body))
}

// unpackInputs returns the source cases for Unpack vectors and fuzz seeds.
//
// Returns:
//   - []struct{name string; b []byte}: the Unpack source cases.
func unpackInputs() []struct {
	name string
	b    []byte
} {
	return append(unpackPositiveInputs(), unpackNegativeInputs()...)
}

// unpackPositiveInputs returns source cases accepted by Unpack.
//
// Returns:
//   - []struct{name string; b []byte}: accepted Unpack source cases.
func unpackPositiveInputs() []struct {
	name string
	b    []byte
} {
	type in = struct {
		name string
		b    []byte
	}
	return []in{
		{"legacy text", []byte("plain text")},
		{"legacy empty", nil},
		{"legacy invalid utf8", []byte{0xff, 0xfe, 'a'}},
		{"short magic is legacy", []byte("GONE2\x00\x00")},
		{"empty message no files", gone2Raw(`{"v":2,"msg":0,"files":[]}`, "")},
		{"message and two files", gone2Raw(`{"v":2,"msg":2,"files":[{"name":"a.txt","type":"text/plain","size":3},{"name":"b","type":"image/png","size":1}]}`, "hixyzP")},
		{"whitespace and key order", gone2Raw(" {\"files\" : [ ],\n\"msg\":1, \"v\":2} ", "a")},
		{"unknown members", gone2Raw(`{"v":2,"msg":0,"x":[1,{}],"files":[{"name":"n","type":"text/csv","size":1,"extra":null}]}`, "z")},
		{"duplicate keys last wins", gone2Raw(`{"v":1,"v":2,"msg":5,"msg":0,"files":null,"files":[]}`, "")},
		{"number forms", gone2Raw(`{"v":2.0,"msg":1e0,"files":[{"name":"n","type":"t","size":-0},{"name":"m","type":"t","size":1e-4000},{"name":"o","type":"t","size":10E-1}]}`, "ab")},
		{"escaped member name", gone2Raw(`{"\u0076":2,"msg":0,"files":[]}`, "")},
		{"lone surrogates in name", gone2Raw(`{"v":2,"msg":0,"files":[{"name":"\udc00\ud800","type":"","size":0}]}`, "")},
		{"unsafe metadata resanitized", gone2Raw(`{"v":2,"msg":0,"files":[{"name":"..\\..\\x\u202e.exe","type":"TEXT/HTML","size":0}]}`, "")},
		{"bom in message kept in bytes", gone2Raw(`{"v":2,"msg":4,"files":[]}`, "\ufeffa")},
	}
}

// unpackNegativeInputs returns source cases rejected by Unpack.
//
// Returns:
//   - []struct{name string; b []byte}: rejected Unpack source cases.
func unpackNegativeInputs() []struct {
	name string
	b    []byte
} {
	f := func(s string) string { return `{"v":2,"msg":0,"files":[{"name":"n","type":"t","size":` + s + `}]}` }
	eleven := `{"v":2,"msg":0,"files":[` + strings.TrimSuffix(strings.Repeat(`{"name":"n","type":"t","size":0},`, 11), ",") + `]}`
	type in = struct {
		name string
		b    []byte
	}
	return []in{
		{"truncated prefix", []byte("GONE2\x00\x00\x00\x00\x00")},
		{"header length past end", append(append(Magic[:], 0, 0, 0, 30), `{"v":2,"msg":0,"files":[]}`...)},
		{"header length over cap", append(append(Magic[:], 0, 0, 0x40, 1), make([]byte, MaxHeaderBytes+1)...)},
		{"bom before header", gone2Raw("\ufeff"+`{"v":2,"msg":0,"files":[]}`, "")},
		{"invalid utf8 in header", gone2Raw(`{"v":2,"msg":0,"files":[],"x":"`+"\xc3"+`"}`, "")},
		{"header null", gone2Raw("null", "")},
		{"header array", gone2Raw("[]", "")},
		{"trailing data after json", gone2Raw(`{"v":2,"msg":0,"files":[]} 1`, "")},
		{"v is 1", gone2Raw(`{"v":1,"msg":0,"files":[]}`, "")},
		{"v is string", gone2Raw(`{"v":"2","msg":0,"files":[]}`, "")},
		{"uppercase V", gone2Raw(`{"V":2,"msg":0,"files":[]}`, "")},
		{"msg fractional", gone2Raw(`{"v":2,"msg":0.5,"files":[]}`, "")},
		{"msg negative", gone2Raw(`{"v":2,"msg":-1,"files":[]}`, "")},
		{"msg overflow", gone2Raw(`{"v":2,"msg":1e400,"files":[]}`, "")},
		{"size 2^53", gone2Raw(f("9007199254740992"), "")},
		{"size string", gone2Raw(f(`"0"`), "")},
		{"files object", gone2Raw(`{"v":2,"msg":0,"files":{}}`, "")},
		{"files missing", gone2Raw(`{"v":2,"msg":0}`, "")},
		{"file entry null", gone2Raw(`{"v":2,"msg":0,"files":[null]}`, "")},
		{"file name number", gone2Raw(`{"v":2,"msg":0,"files":[{"name":1,"type":"t","size":0}]}`, "")},
		{"file type missing", gone2Raw(`{"v":2,"msg":0,"files":[{"name":"n","size":0}]}`, "")},
		{"eleven files", gone2Raw(eleven, "")},
		{"sizes below data", gone2Raw(f("1"), "ab")},
		{"sizes above data", gone2Raw(f("3"), "ab")},
		{"sizes sum overflow", gone2Raw(`{"v":2,"msg":9007199254740991,"files":[{"name":"n","type":"t","size":9007199254740991}]}`, "")},
	}
}

// buildUnpack computes Unpack vector outputs for each source case.
//
// Returns:
//   - []unpackCase: the completed Unpack vector cases.
func buildUnpack() []unpackCase {
	ins := unpackInputs()
	out := make([]unpackCase, len(ins))
	for i, in := range ins {
		c := unpackCase{Name: in.name, Input: hx(in.b)}
		p, err := Unpack(in.b)
		if err != nil {
			c.Error = errCode(err)
		} else {
			c.Message, c.Files = hx(p.Message), toFileVecs(p.Files)
		}
		out[i] = c
	}
	return out
}
