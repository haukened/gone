package envelope

import (
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

const testManageToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestNewManageLink(t *testing.T) {
	id := domain.SecretID(strings.Repeat("ab", 16))
	l, err := NewManageLink("https://gone.example", id, testManageToken)
	if err != nil {
		t.Fatalf("NewManageLink: %v", err)
	}
	want := "https://gone.example/manage/" + id.String() + "#" + testManageToken
	if l.String() != want {
		t.Fatalf("String = %q, want %q", l.String(), want)
	}
	cases := []struct {
		origin string
		id     domain.SecretID
		token  string
	}{
		{"https://gone.example/", id, testManageToken},
		{"ftp://gone.example", id, testManageToken},
		{"https://gone.example?x", id, testManageToken},
		{"%%", id, testManageToken},
		{"https://gone.example", "nope", testManageToken},
		{"https://gone.example", id, "short"},
		{"https://gone.example", id, testManageToken[:42] + "%"},
	}
	for _, c := range cases {
		if _, err := NewManageLink(c.origin, c.id, c.token); !errors.Is(err, ErrInvalidManageLink) {
			t.Errorf("NewManageLink(%q, %q, %q) = %v", c.origin, c.id, c.token, err)
		}
	}
}

// TestParseManageLink checks a valid manage link parses and round-trips.
//
// Parameters:
//   - t: the test.
func TestParseManageLink(t *testing.T) {
	id := strings.Repeat("0f", 16)
	good := "https://gone.example/manage/" + id + "#" + testManageToken
	l, err := ParseManageLink(good)
	if err != nil || l.Origin != "https://gone.example" || l.ID.String() != id || l.Token.String() != testManageToken {
		t.Fatalf("ParseManageLink: %+v %v", l, err)
	}
	if l.String() != good {
		t.Fatalf("round trip %q", l.String())
	}
	if _, err := ParseManageLink("http://localhost:8080/manage/" + id + "#" + testManageToken); err != nil {
		t.Errorf("http localhost rejected: %v", err)
	}
}

// TestParseManageLinkRejects checks that malformed manage links are
// rejected with ErrInvalidManageLink.
//
// Parameters:
//   - t: the test.
func TestParseManageLinkRejects(t *testing.T) {
	id := strings.Repeat("0f", 16)
	tok := testManageToken
	bad := []string{
		"https://gone.example/manage/" + id,
		"https://gone.example/manage/" + id + "#",
		"https://gone.example/manage/" + id + "#" + tok[:42],
		"https://gone.example/manage/" + id + "#" + tok + "A",
		"https://gone.example/manage/" + id + "#" + tok[:41] + "%41",
		"https://gone.example/manage/" + id + "#" + tok + "#x",
		"https://gone.example/manage/" + id + "#" + tok[:42] + "=",
		"https://gone.example/manage/" + strings.ToUpper(id) + "#" + tok,
		"https://gone.example/manage/" + id + "/#" + tok,
		"https://gone.example/secret/" + id + "#" + tok,
		"https://gone.example/app/manage/" + id + "#" + tok,
		"https://gone.example/manage/" + id + "?a=b#" + tok,
		"https://gone.example/manage/" + id + "?#" + tok,
		"https://gone.example/manage%2F" + id + "#" + tok,
		"https://user@gone.example/manage/" + id + "#" + tok,
		"javascript:alert(1)//manage/" + id + "#" + tok,
		"/manage/" + id + "#" + tok,
		"https:///manage/" + id + "#" + tok,
		"%%",
	}
	for _, in := range bad {
		if _, err := ParseManageLink(in); !errors.Is(err, ErrInvalidManageLink) {
			t.Errorf("ParseManageLink(%q) = %v", in, err)
		}
	}
}

func FuzzParseManageLink(f *testing.F) {
	id := strings.Repeat("0f", 16)
	f.Add("https://gone.example/manage/" + id + "#" + testManageToken)
	f.Add("http://localhost:8080/manage/" + id + "#" + testManageToken)
	f.Add("https://gone.example/manage/" + id + "#%41")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		l, err := ParseManageLink(s)
		if err != nil {
			if !errors.Is(err, ErrInvalidManageLink) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if !l.ID.Valid() {
			t.Fatalf("accepted invalid id %q", l.ID)
		}
		if _, err := domain.ParseManageToken(l.Token.String()); err != nil {
			t.Fatalf("accepted invalid token %q", l.Token)
		}
		again, err := ParseManageLink(l.String())
		if err != nil || again != l {
			t.Fatalf("round trip mismatch %q: %v", l.String(), err)
		}
	})
}
