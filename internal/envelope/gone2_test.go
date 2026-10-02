package envelope

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// raw builds GONE2 bytes from a header string and body without validation.
func raw(header string, body []byte) []byte {
	out := append([]byte{}, Magic[:]...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(header)))
	out = append(out, header...)
	return append(out, body...)
}

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

func TestBuildHeader(t *testing.T) {
	big := []fileHeader{{name: strings.Repeat("a", MaxHeaderBytes)}}
	if _, err := buildHeader(0, big); !errors.Is(err, ErrHeaderTooLarge) {
		t.Fatalf("oversized: %v", err)
	}
	if h, err := buildHeader(0, nil); err != nil || string(h) != `{"v":2,"msg":0,"files":[]}` {
		t.Fatalf("empty: %q %v", h, err)
	}
}

// TestWorstCaseHeaderFits pins the invariant that sanitized input can never
// reach MaxHeaderBytes, so Pack only fails on it if limits change.
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

func TestRoundTrip(t *testing.T) {
	ps := []Payload{
		{Message: []byte("text only")},
		{Message: []byte{}, Files: []File{{Name: "a", Type: "image/png", Data: []byte{1, 2, 3}}}},
		{Message: []byte("m"), Files: []File{{Name: "x.bin", Type: DefaultType, Data: []byte{}}, {Name: "y", Type: "text/csv", Data: []byte("1,2")}}},
		{Message: Magic[:]},
	}
	for i, p := range ps {
		b, err := Pack(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Unpack(b)
		if err != nil || !bytes.Equal(got.Message, p.Message) || len(got.Files) != len(p.Files) {
			t.Fatalf("%d: %+v %v", i, got, err)
		}
		for j, f := range p.Files {
			g := got.Files[j]
			if g.Name != f.Name || g.Type != f.Type || !bytes.Equal(g.Data, f.Data) {
				t.Errorf("%d/%d: %+v", i, j, g)
			}
		}
	}
}
