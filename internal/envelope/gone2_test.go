package envelope

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

var roundTripPayloads = []Payload{
	{Message: []byte("text only")},
	{Message: []byte{}, Files: []File{{Name: "a", Type: "image/png", Data: []byte{1, 2, 3}}}},
	{Message: []byte("m"), Files: []File{{Name: "x.bin", Type: DefaultType, Data: []byte{}}, {Name: "y", Type: "text/csv", Data: []byte("1,2")}}},
	{Message: Magic[:]},
}

// raw builds GONE2 bytes from a header string and body without validation.
//
// Parameters:
//   - header: the JSON header to encode.
//   - body: the body bytes to append after the header.
//
// Returns:
//   - []byte: the encoded GONE2 test envelope bytes.
func raw(header string, body []byte) []byte {
	out := append([]byte{}, Magic[:]...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(header)))
	out = append(out, header...)
	return append(out, body...)
}

// TestPackTextOnly verifies legacy text-only payload packing behavior.
//
// Parameters:
//   - t: the test handle.
func TestPackTextOnly(t *testing.T) {
	for _, msg := range [][]byte{nil, []byte(""), []byte("hello"), []byte("GONE2"), []byte("GONE2\x00\x00")} {
		got, err := Pack(Payload{Message: msg})
		if err != nil || !bytes.Equal(got, msg) {
			t.Fatalf("Pack(%q) = %q, %v", msg, got, err)
		}
	}
	msg := []byte("abc")
	got, _ := Pack(Payload{Message: msg})
	got[0] = 'z'
	if msg[0] != 'a' {
		t.Fatal("Pack must not alias the message")
	}
}

// TestPackCanonical verifies canonical GONE2 packing output.
//
// Parameters:
//   - t: the test handle.
func TestPackCanonical(t *testing.T) {
	p := Payload{
		Message: []byte("hi"),
		Files: []File{
			{Name: "../a.txt", Type: "Text/Plain; charset=utf-8", Data: []byte("xyz")},
			{Name: "", Type: "text/html", Data: nil},
		},
	}
	got, err := Pack(p)
	if err != nil {
		t.Fatal(err)
	}
	want := raw(`{"v":2,"msg":2,"files":[{"name":"a.txt","type":"text/plain","size":3},{"name":"file","type":"application/octet-stream","size":0}]}`, []byte("hixyz"))
	if !bytes.Equal(got, want) {
		t.Fatalf("Pack =\n%q\nwant\n%q", got, want)
	}
	magic := Magic[:]
	got, err = Pack(Payload{Message: magic})
	if err != nil || !bytes.Equal(got, raw(`{"v":2,"msg":8,"files":[]}`, magic)) {
		t.Fatalf("magic text: %q %v", got, err)
	}
}

// TestPackErrors verifies Pack rejects invalid payloads.
//
// Parameters:
//   - t: the test handle.
func TestPackErrors(t *testing.T) {
	cases := []struct {
		name string
		p    Payload
		want error
	}{
		{"too many", Payload{Files: make([]File, MaxFiles+1)}, ErrTooManyFiles},
		{"bad name", Payload{Files: []File{{Name: "\xff"}}}, ErrInvalidMetadata},
		{"bad type", Payload{Files: []File{{Type: "\xff"}}}, ErrInvalidMetadata},
	}
	for _, c := range cases {
		if _, err := Pack(c.p); !errors.Is(err, c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// TestBuildHeader verifies header size limits and empty header output.
//
// Parameters:
//   - t: the test handle.
func TestBuildHeader(t *testing.T) {
	big := []fileHeader{{name: strings.Repeat("a", MaxHeaderBytes)}}
	if _, err := buildHeader(0, big); !errors.Is(err, ErrHeaderTooLarge) {
		t.Fatalf("oversized: %v", err)
	}
	if h, err := buildHeader(0, nil); err != nil || string(h) != `{"v":2,"msg":0,"files":[]}` {
		t.Fatalf("empty: %q %v", h, err)
	}
}

// TestWorstCaseHeaderFits pins that sanitized input cannot reach MaxHeaderBytes.
//
// Parameters:
//   - t: the test handle.
func TestWorstCaseHeaderFits(t *testing.T) {
	for _, name := range []string{strings.Repeat("\U0001F600", maxNameRunes), strings.Repeat(`"`, maxNameRunes)} {
		fs := make([]File, MaxFiles)
		for i := range fs {
			fs[i] = File{Name: name, Type: "application/x-7z-compressed"}
		}
		b, err := Pack(Payload{Message: []byte("m"), Files: fs})
		if err != nil {
			t.Fatal(err)
		}
		if h := binary.BigEndian.Uint32(b[8:12]); h > MaxHeaderBytes*3/4 {
			t.Fatalf("worst-case header %d bytes is too close to the cap", h)
		}
	}
}

// TestRoundTrip verifies Pack and Unpack preserve representative payloads.
//
// Parameters:
//   - t: the test handle.
func TestRoundTrip(t *testing.T) {
	for i, p := range roundTripPayloads {
		checkGone2RoundTrip(t, i, p)
	}
}

// checkGone2RoundTrip verifies one Pack and Unpack round trip case.
//
// Parameters:
//   - t: the test handle.
//   - i: the case index.
//   - p: the payload to round trip.
func checkGone2RoundTrip(t *testing.T, i int, p Payload) {
	t.Helper()
	b, err := Pack(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unpack(b)
	if err != nil || !bytes.Equal(got.Message, p.Message) || len(got.Files) != len(p.Files) {
		t.Fatalf("%d: %+v %v", i, got, err)
	}
	checkGone2RoundTripFiles(t, i, got.Files, p.Files)
}

// checkGone2RoundTripFiles verifies round-tripped file metadata and data.
//
// Parameters:
//   - t: the test handle.
//   - i: the case index.
//   - got: the decoded files.
//   - want: the original files.
func checkGone2RoundTripFiles(t *testing.T, i int, got []File, want []File) {
	t.Helper()
	for j, f := range want {
		g := got[j]
		if g.Name != f.Name || g.Type != f.Type || !bytes.Equal(g.Data, f.Data) {
			t.Errorf("%d/%d: %+v", i, j, g)
		}
	}
}
