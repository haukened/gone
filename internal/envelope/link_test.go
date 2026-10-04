package envelope

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

// TestNewLink verifies links render correctly and invalid inputs fail.
//
// Parameters:
//   - t: the test handle.
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
	checkNewLinkRejects(t, id, f)
}

// checkNewLinkRejects verifies invalid NewLink inputs fail.
//
// Parameters:
//   - t: the test handle.
//   - id: the baseline valid secret ID.
//   - f: the baseline valid fragment.
func checkNewLinkRejects(t *testing.T, id domain.SecretID, f Fragment) {
	t.Helper()
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
		{"******gone.example", id, f},
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

// TestParseLink verifies link parsing, rendering, and validation.
//
// Parameters:
//   - t: the test handle.
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
	checkParseLinkCases(t, id, f.String())
}

// checkParseLinkCases verifies additional ParseLink cases.
//
// Parameters:
//   - t: the test handle.
//   - id: the baseline valid secret ID string.
//   - frag: the baseline valid fragment string.
func checkParseLinkCases(t *testing.T, id string, frag string) {
	t.Helper()
	cases := []struct {
		in   string
		want error
	}{
		{"http://localhost:8080/secret/" + id + "#" + frag, nil},
		{"https://gone.example/secret/" + id, ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#", ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#v1%3A" + frag[3:], ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#" + frag + "#x", ErrInvalidFragment},
		{"https://gone.example/secret/" + id + "#v3:" + frag[3:], domain.ErrInvalidVersion},
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
