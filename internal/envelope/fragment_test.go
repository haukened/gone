package envelope

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

var testKey = bytes.Repeat([]byte{0xab}, domain.KeySize)

func TestNewFragment(t *testing.T) {
	f, err := NewFragment(1, testKey)
	if err != nil || f.Version() != 1 || !bytes.Equal(f.Key(), testKey) {
		t.Fatalf("NewFragment: %v", err)
	}
	k := f.Key()
	k[0] = 0
	if f.Key()[0] != 0xab {
		t.Fatal("Key must return a copy")
	}
	if _, err := NewFragment(2, testKey); !errors.Is(err, domain.ErrInvalidVersion) {
		t.Fatalf("v2: %v", err)
	}
	if _, err := NewFragment(1, testKey[:31]); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("short key: %v", err)
	}
}

func TestFragmentRoundTrip(t *testing.T) {
	f, _ := NewFragment(1, testKey)
	s := f.String()
	if !strings.HasPrefix(s, "v1:") || len(s) != 46 {
		t.Fatalf("String = %q", s)
	}
	g, err := ParseFragment(s)
	if err != nil || g.Version() != 1 || !bytes.Equal(g.Key(), testKey) {
		t.Fatalf("ParseFragment: %v", err)
	}
}

func TestParseFragmentErrors(t *testing.T) {
	key := domain.EncodeB64URL(testKey)
	cases := []struct {
		in   string
		want error
	}{
		{"", ErrInvalidFragment},
		{"#v1:" + key, ErrInvalidFragment},
		{"V1:" + key, ErrInvalidFragment},
		{"v:" + key, ErrInvalidFragment},
		{"v01:" + key, ErrInvalidFragment},
		{"v0:" + key, ErrInvalidFragment},
		{"v1000:" + key, ErrInvalidFragment},
		{"v256:" + key, ErrInvalidFragment},
		{"v+1:" + key, ErrInvalidFragment},
		{"v1" + key, ErrInvalidFragment},
		{"v1:", ErrInvalidFragment},
		{"v1%3A" + key, ErrInvalidFragment},
		{"%761:" + key, ErrInvalidFragment},
		{"v1:" + key + "%zz", ErrInvalidFragment},
		{"v1:" + key + "#", ErrInvalidFragment},
		{"v1:" + key + "=", ErrInvalidFragment},
		{"v1:" + key[:42], ErrInvalidFragment},
		{"v1:" + key + "A", ErrInvalidFragment},
		{"v1:" + key[:42] + "B", ErrInvalidFragment},
		{"v1:" + domain.EncodeB64URL(testKey[:31]), ErrInvalidFragment},
		{"v1:" + strings.Repeat("A", 600), ErrInvalidFragment},
		{"v2:" + key, domain.ErrInvalidVersion},
		{"v255:" + key, domain.ErrInvalidVersion},
		{"v2:%%", ErrInvalidFragment},
	}
	for _, c := range cases {
		if _, err := ParseFragment(c.in); !errors.Is(err, c.want) {
			t.Errorf("ParseFragment(%q) = %v, want %v", c.in, err, c.want)
		}
	}
}

func TestNewLink(t *testing.T) {
	f, _ := NewFragment(1, testKey)
	id := domain.SecretID(strings.Repeat("a", 32))
	l, err := NewLink("https://gone.example:8443", id, f)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://gone.example:8443/secret/" + id.String() + "#" + f.String()
	if l.String() != want {
		t.Fatalf("String = %q", l.String())
	}
	bad := []struct {
		origin string
		id     domain.SecretID
		f      Fragment
	}{
		{"https://gone.example/", id, f},
		{"https://gone.example/app", id, f},
		{"https://gone.example?x", id, f},
		{"https://gone.example?", id, f},
		{"https://gone.example#x", id, f},
		{"https://u:p@gone.example", id, f},
		{"ftp://gone.example", id, f},
		{"gone.example", id, f},
		{"https://", id, f},
		{"://bad", id, f},
		{"https://gone.example", "nope", f},
		{"https://gone.example", id, Fragment{}},
	}
	for _, c := range bad {
		if _, err := NewLink(c.origin, c.id, c.f); !errors.Is(err, ErrInvalidLink) {
			t.Errorf("NewLink(%q, %q) = %v", c.origin, c.id, err)
		}
	}
}

func TestParseLink(t *testing.T) {
	f, _ := NewFragment(1, testKey)
	id := strings.Repeat("0f", 16)
	good := "https://gone.example/secret/" + id + "#" + f.String()
	l, err := ParseLink(good)
	if err != nil || l.Origin != "https://gone.example" || l.ID.String() != id || !bytes.Equal(l.Fragment.Key(), testKey) {
		t.Fatalf("ParseLink: %+v %v", l, err)
	}
	if l.String() != good {
		t.Fatalf("round trip %q", l.String())
	}
	frag := f.String()
	cases := []struct {
		in   string
		want error
	}{
		{"http://localhost:8080/secret/" + id + "#" + frag, nil},
		{"https://gone.example/secret/" + id, ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#", ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#v1%3A" + frag[3:], ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#" + frag + "#x", ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#v2:" + frag[3:], domain.ErrInvalidVersion},
		{"https://gone.example/secret/" + id + "#" + frag + "%zz", ErrInvalidLink},
		{"https://gone.example/secret/" + strings.ToUpper(id) + "#" + frag, ErrInvalidLink},
		{"https://gone.example/secret/" + id + "/#" + frag, ErrInvalidLink},
		{"https://gone.example/app/secret/" + id + "#" + frag, ErrInvalidLink},
		{"https://gone.example/secret/" + id + "?a=b#" + frag, ErrInvalidLink},
		{"https://gone.example/secret/" + id + "?#" + frag, ErrInvalidLink},
		{"https://gone.example/secret%2F" + id + "#" + frag, ErrInvalidLink},
		{"https://user@gone.example/secret/" + id + "#" + frag, ErrInvalidLink},
		{"javascript:alert(1)//secret/" + id + "#" + frag, ErrInvalidLink},
		{"/secret/" + id + "#" + frag, ErrInvalidLink},
		{"https:///secret/" + id + "#" + frag, ErrInvalidLink},
		{"%%", ErrInvalidLink},
	}
	for _, c := range cases {
		if _, err := ParseLink(c.in); !errors.Is(err, c.want) {
			t.Errorf("ParseLink(%q) = %v, want %v", c.in, err, c.want)
		}
	}
}
