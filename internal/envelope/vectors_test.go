package envelope

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

// update regenerates test/vectors from the inputs below. Run:
//
//	go test ./internal/envelope -run TestVectors -update
var update = flag.Bool("update", false, "regenerate test/vectors")

const vectorDir = "../../test/vectors"

// errCode maps package errors to the language-neutral codes in vectors.
func errCode(err error) string {
	codes := []struct {
		err  error
		code string
	}{
		{ErrTooManyFiles, "too_many_files"},
		{ErrInvalidEnvelope, "invalid_envelope"},
		{domain.ErrInvalidVersion, "unsupported_version"},
		{ErrInvalidFragment, "invalid_fragment"},
		{ErrDecrypt, "decrypt"},
	}
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return c.code
		}
	}
	return err.Error()
}

// seq returns n bytes counting up from start.
func seq(start byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = start + byte(i)
	}
	return b
}

func hx(b []byte) string { return hex.EncodeToString(b) }

type aeadCase struct {
	Name       string `json:"name"`
	Key        string `json:"key"`
	Nonce      string `json:"nonce"`
	Plaintext  string `json:"plaintext,omitempty"`
	Ciphertext string `json:"ciphertext"`
	Error      string `json:"error,omitempty"`
}

type aeadVectors struct {
	Description string     `json:"description"`
	AAD         string     `json:"aad"`
	Cases       []aeadCase `json:"cases"`
}

func buildAEAD(t *testing.T) aeadVectors {
	key, nonce := seq(0, domain.KeySize), seq(0xa0, domain.NonceSize)
	pts := []struct {
		name string
		pt   []byte
	}{
		{"empty", nil},
		{"text", []byte("gone vector")},
		{"two blocks plus", seq(0x40, 33)},
	}
	v := aeadVectors{Description: "AES-256-GCM v1; hex fields; AAD is UTF-8 text", AAD: domain.AADv1}
	var ct []byte
	for _, p := range pts {
		gotNonce, c, err := sealFrom(bytes.NewReader(nonce), key, p.pt)
		if err != nil || !bytes.Equal(gotNonce, nonce) {
			t.Fatalf("seal %s: %v", p.name, err)
		}
		ct = c
		v.Cases = append(v.Cases, aeadCase{Name: p.name, Key: hx(key), Nonce: hx(nonce), Plaintext: hx(p.pt), Ciphertext: hx(c)})
	}
	return appendAEADNegatives(v, key, nonce, ct)
}

func appendAEADNegatives(v aeadVectors, key, nonce, ct []byte) aeadVectors {
	flip := func(b []byte, i int) []byte { c := bytes.Clone(b); c[i] ^= 0x01; return c }
	other := seq(1, domain.KeySize)
	noAAD, _ := newAEAD(key)
	neg := []struct {
		name           string
		key, nonce, ct []byte
	}{
		{"flipped tag bit", key, nonce, flip(ct, len(ct)-1)},
		{"flipped ciphertext bit", key, nonce, flip(ct, 0)},
		{"wrong aad", key, nonce, noAAD.Seal(nil, nonce, seq(0x40, 33), nil)},
		{"wrong key", other, nonce, ct},
		{"short nonce", key, nonce[:11], ct},
		{"31-byte key", key[:31], nonce, ct},
		{"truncated tag", key, nonce, ct[:domain.TagSize-1]},
	}
	for _, n := range neg {
		_, err := Open(n.key, n.nonce, n.ct)
		v.Cases = append(v.Cases, aeadCase{Name: n.name, Key: hx(n.key), Nonce: hx(n.nonce), Ciphertext: hx(n.ct), Error: errCode(err)})
	}
	return v
}

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

func toFileVecs(fs []File) []fileVec {
	out := make([]fileVec, len(fs))
	for i, f := range fs {
		out[i] = fileVec{Name: f.Name, Type: f.Type, Data: hx(f.Data)}
	}
	return out
}

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
		{Name: "sanitized metadata", Message: "x", Files: []fileVec{
			{"../../etc/passwd", "Text/Plain; charset=utf-8", "00"},
			{"invoice\u202etxt.exe", "text/html", "01"},
			{"\u200b..\u200b", "IMAGE/PNG", ""},
			{"quote\"back\\slash\ttab", "image/svg+xml", "02"},
			{"日本😀.txt", "text/csv", "03"},
		}},
		{Name: "ten files", Files: tenFiles},
		{Name: "eleven files", Files: append(append([]fileVec{}, tenFiles...), fileVec{"x", "", ""})},
	}
}

func buildPack(t *testing.T) []packCase {
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

func gone2Raw(header string, body string) []byte {
	return raw(header, []byte(body))
}

func unpackInputs() []struct {
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

type fragmentCase struct {
	Input   string `json:"input"`
	Version int    `json:"version,omitempty"`
	Key     string `json:"key,omitempty"`
	Error   string `json:"error,omitempty"`
}

func fragmentInputs() []string {
	k := domain.EncodeB64URL(seq(0, domain.KeySize))
	zeros := strings.Repeat("A", 43)
	return []string{
		"v1:" + k, "v1:" + zeros, "v1:" + strings.Repeat("_", 42) + "w",
		"", "v1:", "v1" + k, "#v1:" + k, "V1:" + k, "x1:" + k, "v:" + k,
		"v0:" + k, "v01:" + k, "v+1:" + k, "v1.0:" + k, "v256:" + k, "v1000:" + k,
		"v2:" + k, "v255:" + k,
		"v1:" + k + "=", "v1:" + strings.Repeat("A", 42) + "B", "v1:" + zeros[:42], "v1:" + zeros + "A",
		"v1:" + zeros[:41] + "+/", "v1:" + zeros[:42] + "%", "v1%3A" + k, "%761:" + k, "v1:" + k + "#", "v1:" + k + " ",
		"v1:" + domain.EncodeB64URL(seq(0, 31)), "v1:" + domain.EncodeB64URL(seq(0, 33)),
	}
}

func buildFragments() []fragmentCase {
	ins := fragmentInputs()
	out := make([]fragmentCase, len(ins))
	for i, s := range ins {
		c := fragmentCase{Input: s}
		f, err := ParseFragment(s)
		if err != nil {
			c.Error = errCode(err)
		} else {
			c.Version, c.Key = int(f.Version()), hx(f.Key())
		}
		out[i] = c
	}
	return out
}

type stringCase struct {
	In  string `json:"in"`
	Out string `json:"out"`
}

type sanitizeVectors struct {
	Names []stringCase `json:"names"`
	Types []stringCase `json:"types"`
}

func buildSanitize() sanitizeVectors {
	names := []string{
		"report.pdf", "", ".", "..", "../../etc/passwd", `..\..\x`, `C:\Users\a\evil.exe`, "dir/", "/", "a/..",
		"  name.txt \t", "\u00a0\u3000x\ufeff", "a\x00b\x1fc\x7fd\u0085e", "invoice\u202etxt.exe",
		"a\u200bb\u200fc\u2066d\u206fe\u061cf\u180eg", "\u200b..\u200b", "\u2028x\u2029", "\u1680\u205fy\u202f",
		strings.Repeat("é", 300), strings.Repeat("😀", 300), strings.Repeat("\u200b", 300) + "ok",
		strings.Repeat("a", 254) + " b", "日本語.txt", "\ufffd",
	}
	types := []string{
		"application/pdf", "Image/PNG", " text/plain ; charset=utf-8", "text/plain;", "text/html",
		"image/svg+xml", "", "application/x-7z-compressed", "text/plaın", "TEXT/PLAİN", "\u00a0text/csv\u3000",
		"text/csv\x00", "\ttext/csv\n", "video/mp4;codecs=avc1",
	}
	v := sanitizeVectors{}
	for _, n := range names {
		v.Names = append(v.Names, stringCase{n, SanitizeFileName(n)})
	}
	for _, ty := range types {
		v.Types = append(v.Types, stringCase{ty, SafeType(ty)})
	}
	return v
}

type acceptCase struct {
	In string `json:"in"`
	OK bool   `json:"ok"`
}

type serverVectors struct {
	Versions []acceptCase `json:"versions"`
	Nonces   []acceptCase `json:"nonces"`
}

func buildServer() serverVectors {
	versions := []string{"1", "2", "0", "01", "+1", "-1", " 1", "1 ", "1.0", "256", "1000", "", "a", "１"}
	nonces := []string{
		"AAAAAAAAAAAAAAAA", "oKGio6Slpqeoqaqr", "____________----", "AAAAAAAAAAAAAAA", "AAAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAA=", "AAAAAAAAAAAAAAA+", "AAAAAAAAAAAAAAA/", "AAAAAAAAAAAAAAA ", "", "AAAAAAAAAAAAAAAAAAAA",
	}
	v := serverVectors{}
	for _, s := range versions {
		_, err := domain.ParseVersion(s)
		v.Versions = append(v.Versions, acceptCase{s, err == nil})
	}
	for _, s := range nonces {
		v.Nonces = append(v.Nonces, acceptCase{s, domain.ValidateProtocol(domain.ProtocolV1, s) == nil})
	}
	return v
}

// marshalVector encodes v as stable, human-diffable JSON.
func marshalVector(t *testing.T, v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestVectors checks that the committed vectors match what the Go
// implementation produces today, or rewrites them with -update.
func TestVectors(t *testing.T) {
	files := map[string]any{
		"aead_v1.json":        buildAEAD(t),
		"envelope_gone2.json": envelopeVectors{Description: "GONE2 plaintext; binary fields are hex", Pack: buildPack(t), Unpack: buildUnpack()},
		"fragment_v1.json":    buildFragments(),
		"sanitize.json":       buildSanitize(),
		"server_headers.json": buildServer(),
	}
	for name, v := range files {
		path := filepath.Join(vectorDir, name)
		want := marshalVector(t, v)
		if *update {
			if err := os.WriteFile(path, want, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path) // #nosec G304 -- fixed test path
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; regenerate with -update and review the diff", name)
		}
	}
}
