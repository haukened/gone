package integration_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// gone2 frames a raw header and body as GONE2 plaintext.
//
// Parameters:
//   - header: header bytes, used as given.
//   - body: message and file bytes.
//
// Returns the plaintext.
func gone2(header string, body string) []byte {
	out := append([]byte{}, envelope.Magic[:]...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(header))) // #nosec G115 -- test headers are small
	return append(append(out, header...), body...)
}

// padHeader returns `{"v":2,"msg":0,"files":[],"pad":"xx..."}` padded to
// exactly n bytes.
//
// Parameters:
//   - n: header length; at least 37.
//
// Returns the header.
func padHeader(n int) string {
	head := `{"v":2,"msg":0,"files":[],"pad":"`
	return head + strings.Repeat("x", n-len(head)-2) + `"}`
}

// nestedHeader returns a valid header whose unknown member "x" nests
// arrays as deep as fits in n bytes.
//
// Parameters:
//   - n: header length budget.
//
// Returns the header, at most n bytes.
func nestedHeader(n int) string {
	head, tail := `{"v":2,"msg":0,"files":[],"x":`, `}`
	depth := (n - len(head) - len(tail)) / 2
	return head + strings.Repeat("[", depth) + strings.Repeat("]", depth) + tail
}

// nestedObjects returns a valid header whose unknown member nests objects
// {"a":{"a":...}} as deep as fits in n bytes.
//
// Parameters:
//   - n: header length budget.
//
// Returns the header, at most n bytes.
func nestedObjects(n int) string {
	head, tail := `{"v":2,"msg":0,"files":[],"x":`, `0}`
	depth := (n - len(head) - len(tail)) / len(`{"a":}`)
	return head + strings.Repeat(`{"a":`, depth) + "0" + strings.Repeat("}", depth) + "}"
}

// filesHeader lists n one-byte text files.
//
// Parameters:
//   - n: number of files.
//
// Returns the header.
func filesHeader(n int) string {
	files := make([]string, n)
	for i := range files {
		files[i] = `{"name":"f.txt","type":"text/plain","size":1}`
	}
	return `{"v":2,"msg":0,"files":[` + strings.Join(files, ",") + `]}`
}

// fileHeader is a header with an empty message and one file entry.
//
// Parameters:
//   - entry: the raw JSON of the entry.
//
// Returns the header.
func fileHeader(entry string) string {
	return `{"v":2,"msg":0,"files":[` + entry + `]}`
}

// bom is U+FEFF, the byte-order mark.
const bom = string(rune(0xfeff))

// esc returns the JSON escape backslash-u followed by four hex digits.
//
// Parameters:
//   - hex: four hex digits.
//
// Returns the escape text.
func esc(hex string) string {
	return string('\\') + "u" + hex
}

// envelopeCase decodes plaintext with Unpack and with envelope.js decode.
//
// Parameters:
//   - name: case name.
//   - pt: the plaintext.
//
// Returns the case.
func envelopeCase(name string, pt []byte) diffCase {
	return diffCase{
		name:    name,
		goV:     func(*testing.T) verdict { return goPayload(pt, nil) },
		js:      diffReq{Op: "decode", Body: pt},
		jsValue: jsPayload,
	}
}

// envelopeCases are GONE2 header boundaries (docs/protocol.md §5.2, §5.3).
//
// Returns the cases.
func envelopeCases() []diffCase {
	msg := func(n string) string { return `{"v":2,"msg":` + n + `,"files":[]}` }
	size := func(n string) string { return fileHeader(`{"name":"a","type":"text/plain","size":` + n + `}`) }
	cases := []diffCase{
		envelopeCase("v 2.0", gone2(`{"v":2.0,"msg":2,"files":[]}`, "hi")),
		envelopeCase("v as string", gone2(`{"v":"2","msg":0,"files":[]}`, "")),
		envelopeCase("V uppercase", gone2(`{"V":2,"msg":0,"files":[]}`, "")),
		envelopeCase("msg 1e0", gone2(msg("1e0"), "x")),
		envelopeCase("msg -0", gone2(msg("-0"), "")),
		envelopeCase("msg 3.0000000000000001 rounds to 3", gone2(msg("3.0000000000000001"), "abc")),
		envelopeCase("msg 1e-400 underflows to 0", gone2(msg("1e-400"), "")),
		envelopeCase("msg 5e-324", gone2(msg("5e-324"), "")),
		envelopeCase("msg 1.5", gone2(msg("1.5"), "ab")),
		envelopeCase("msg 1e400", gone2(msg("1e400"), "")),
		envelopeCase("msg -1", gone2(msg("-1"), "")),
		envelopeCase("msg 2^53+1 rounds to 2^53", gone2(msg("9007199254740993"), "")),
		envelopeCase("size 2^53", gone2(size("9007199254740992"), "")),
		envelopeCase("size 2^53-1 valid but bytes cannot match", gone2(size("9007199254740991"), "")),
		envelopeCase("size one short", gone2(size("1"), "")),
		envelopeCase("duplicate msg last wins", gone2(`{"v":2,"msg":99,"files":[],"msg":3}`, "abc")),
		envelopeCase("duplicate msg last invalid", gone2(`{"v":2,"msg":3,"files":[],"msg":-1}`, "abc")),
		envelopeCase("duplicate files last wins", gone2(`{"v":2,"msg":0,"files":[{"name":"a","type":"x","size":1}],"files":[]}`, "")),
		envelopeCase("escaped key m\\u0073g", gone2(`{"v":2,"m`+esc("0073")+`g":1,"files":[]}`, "z")),
		envelopeCase("__proto__ holds the fields", gone2(`{"__proto__":{"v":2,"msg":0,"files":[]}}`, "")),
		envelopeCase("__proto__ member ignored", gone2(`{"v":2,"msg":0,"files":[],"__proto__":{"v":3}}`, "")),
		envelopeCase("__proto__ in file entry", gone2(fileHeader(`{"__proto__":{"name":"a"},"type":"text/plain","size":0}`), "")),
		envelopeCase("BOM before JSON", gone2(bom+msg("0"), "")),
		envelopeCase("whitespace around JSON", gone2(" \t\r\n"+msg("1")+"\n ", "w")),
		envelopeCase("lone surrogate in name", gone2(fileHeader(`{"name":"`+esc("d800")+`x","type":"text/plain","size":1}`), "a")),
		envelopeCase("surrogate pair in name", gone2(fileHeader(`{"name":"`+esc("d83d")+esc("de00")+`.txt","type":"text/plain","size":1}`), "a")),
		envelopeCase("NUL escape in name", gone2(fileHeader(`{"name":"a`+esc("0000")+`b","type":"TEXT/PLAIN; x","size":0}`), "")),
		envelopeCase("invalid UTF-8 in header", gone2("{\"v\":2,\"msg\":0,\"files\":[],\"x\":\"\xff\"}", "")),
		envelopeCase("trailing comma", gone2(`{"v":2,"msg":0,"files":[],}`, "")),
		envelopeCase("two JSON texts", gone2(msg("0")+msg("0"), "")),
		envelopeCase("header length 16384", gone2(padHeader(envelope.MaxHeaderBytes), "")),
		envelopeCase("header length 16385", gone2(padHeader(envelope.MaxHeaderBytes+1), "")),
		envelopeCase("header length past the end", gone2(msg("0"), "")[:20]),
		envelopeCase("truncated prefix", envelope.Magic[:]),
		envelopeCase("nested arrays to the cap", gone2(nestedHeader(envelope.MaxHeaderBytes), "")),
		envelopeCase("nested objects to the cap", gone2(nestedObjects(envelope.MaxHeaderBytes), "")),
		envelopeCase("files 10", gone2(filesHeader(10), strings.Repeat("x", 10))),
		envelopeCase("files 11", gone2(filesHeader(11), strings.Repeat("x", 11))),
		envelopeCase("files missing", gone2(`{"v":2,"msg":0}`, "")),
		envelopeCase("files an object", gone2(`{"v":2,"msg":0,"files":{"0":{}}}`, "")),
		envelopeCase("file entry null", gone2(fileHeader(`null`), "")),
		envelopeCase("file name a number", gone2(fileHeader(`{"name":1,"type":"text/plain","size":0}`), "")),
	}
	// encoding/json refuses a non-object top level as JSON; JSON.parse
	// accepts it and the shape check rejects it.
	for _, top := range []string{`[2]`, `null`, `2`} {
		c := envelopeCase("top level "+top+" (Go: json, JS: shape)", gone2(top, ""))
		c.anyReject = true
		cases = append(cases, c)
	}
	return cases
}

// TestDifferentialEnvelope feeds the same GONE2 plaintexts to Unpack and to
// envelope.js and fails when they disagree.
//
// Parameters:
//   - t: the test.
func TestDifferentialEnvelope(t *testing.T) {
	runDiff(t, envelopeCases())
}
