package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewManageTokenRoundTrip(t *testing.T) {
	seen := make(map[ManageToken]struct{})
	for i := 0; i < 64; i++ {
		tok, err := NewManageToken()
		if err != nil {
			t.Fatalf("NewManageToken: %v", err)
		}
		if len(tok) != bearerTokenLen {
			t.Fatalf("len=%d want %d", len(tok), bearerTokenLen)
		}
		got, err := ParseManageToken(tok.String())
		if err != nil || got != tok {
			t.Fatalf("ParseManageToken(%q) = %q, %v", tok, got, err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token generated")
		}
		seen[tok] = struct{}{}
	}
}

func TestParseManageTokenInvalid(t *testing.T) {
	valid, err := NewManageToken()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"short", valid.String()[:42]},
		{"long", valid.String() + "A"},
		{"bad char", "!" + valid.String()[1:]},
		{"padding", valid.String()[:42] + "="},
		{"std alphabet", strings.Repeat("+", bearerTokenLen)},
		{"non-canonical", valid.String()[:42] + "_"},
		{"newline", valid.String()[:42] + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok, err := ParseManageToken(tc.in)
			if !errors.Is(err, ErrInvalidManage) {
				t.Fatalf("expected ErrInvalidManage, got %v", err)
			}
			if tok != "" {
				t.Fatalf("expected empty token, got %q", tok)
			}
		})
	}
}

func TestManageTokenHash(t *testing.T) {
	tok := ManageToken(strings.Repeat("A", bearerTokenLen))
	h := tok.Hash()
	if len(h) != 64 || h != tok.Hash() {
		t.Fatalf("hash not 64-char deterministic: %q", h)
	}
	if ClaimToken(tok).Hash() != h {
		t.Fatalf("manage and claim tokens must share the hash primitive")
	}
	if ManageToken(strings.Repeat("B", bearerTokenLen)).Hash() == h {
		t.Fatalf("distinct tokens produced same hash")
	}
}
