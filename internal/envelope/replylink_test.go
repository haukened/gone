package envelope

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

func TestReplyLinkRoundTrip(t *testing.T) {
	pub := p256Key(t, seq(1, 32)).PublicKey().Bytes()
	f, err := NewReplyFragment(pub, testManageToken)
	if err != nil {
		t.Fatalf("NewReplyFragment: %v", err)
	}
	id := strings.Repeat("ab", 16)
	raw := "https://gone.example/reply/" + id + "#" + f.String()
	l, err := ParseReplyLink(raw)
	if err != nil {
		t.Fatalf("ParseReplyLink: %v", err)
	}
	if l.String() != raw || l.Origin != "https://gone.example" || l.ID.String() != id {
		t.Fatalf("link = %+v", l)
	}
	if !bytes.Equal(l.Fragment.PublicKey, pub) || l.Fragment.Fill.String() != testManageToken {
		t.Fatalf("fragment = %+v", l.Fragment)
	}
}

func TestNewReplyFragmentInvalid(t *testing.T) {
	pub := p256Key(t, seq(1, 32)).PublicKey().Bytes()
	if _, err := NewReplyFragment(pub[:64], testManageToken); !errors.Is(err, ErrInvalidFragment) {
		t.Fatalf("short key: %v", err)
	}
	if _, err := NewReplyFragment(pub, "short"); !errors.Is(err, ErrInvalidFragment) {
		t.Fatalf("short fill: %v", err)
	}
}

func TestParseReplyLinkInvalid(t *testing.T) {
	pub := domain.EncodeB64URL(p256Key(t, seq(1, 32)).PublicKey().Bytes())
	frag := "v3:" + pub + "." + testManageToken
	id := strings.Repeat("ab", 16)
	cases := []struct {
		raw string
		err error
	}{
		{"https://gone.example/secret/" + id + "#" + frag, ErrInvalidLink},
		{"https://gone.example/reply/nope#" + frag, ErrInvalidLink},
		{"https://gone.example/reply/" + id + "?x#" + frag, ErrInvalidLink},
		{"ftp://gone.example/reply/" + id + "#" + frag, ErrInvalidLink},
		{"https://gone.example/reply/" + id + "#v3:" + pub, ErrInvalidFragment},
		{"https://gone.example/reply/" + id + "#v1:" + pub + "." + testManageToken, domain.ErrInvalidVersion},
	}
	for _, c := range cases {
		if _, err := ParseReplyLink(c.raw); !errors.Is(err, c.err) {
			t.Errorf("ParseReplyLink(%q) = %v, want %v", c.raw, err, c.err)
		}
	}
}
