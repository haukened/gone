package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewClaimTokenRoundTrip(t *testing.T) {
	seen := make(map[ClaimToken]struct{})
	for i := 0; i < 64; i++ {
		tok, err := NewClaimToken()
		if err != nil {
			t.Fatalf("NewClaimToken: %v", err)
		}
		if len(tok) != bearerTokenLen {
			t.Fatalf("len=%d want %d", len(tok), bearerTokenLen)
		}
		if _, err := ParseClaimToken(tok.String()); err != nil {
			t.Fatalf("ParseClaimToken(%q): %v", tok, err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token generated")
		}
		seen[tok] = struct{}{}
	}
}

func TestParseClaimTokenInvalid(t *testing.T) {
	valid, err := NewClaimToken()
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
		// Last char must have zero trailing bits for 32 bytes; '_' (63) sets them.
		{"non-canonical", valid.String()[:42] + "_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseClaimToken(tc.in); !errors.Is(err, ErrInvalidClaim) {
				t.Fatalf("expected ErrInvalidClaim, got %v", err)
			}
		})
	}
}

func TestClaimTokenHash(t *testing.T) {
	tok := ClaimToken(strings.Repeat("A", bearerTokenLen))
	h := tok.Hash()
	if len(h) != 64 {
		t.Fatalf("hash len=%d want 64", len(h))
	}
	if h != tok.Hash() {
		t.Fatalf("hash not deterministic")
	}
	other := ClaimToken(strings.Repeat("B", bearerTokenLen))
	if other.Hash() == h {
		t.Fatalf("distinct tokens produced same hash")
	}
	if strings.Contains(h, tok.String()) {
		t.Fatalf("hash leaks token")
	}
}
