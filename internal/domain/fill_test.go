package domain

import (
	"errors"
	"testing"
)

func TestFillTokenRoundTrip(t *testing.T) {
	tok, err := NewFillToken()
	if err != nil {
		t.Fatalf("NewFillToken: %v", err)
	}
	got, err := ParseFillToken(tok.String())
	if err != nil || got != tok {
		t.Fatalf("ParseFillToken(%q) = %q, %v", tok, got, err)
	}
	if len(tok.Hash()) != 64 || tok.Hash() != hashBearerToken(tok.String()) {
		t.Fatalf("Hash = %q", tok.Hash())
	}
}

func TestParseFillTokenInvalid(t *testing.T) {
	for _, in := range []string{"", "short", "!" + string(make([]byte, 42))} {
		if _, err := ParseFillToken(in); !errors.Is(err, ErrInvalidFill) {
			t.Errorf("ParseFillToken(%q) err = %v", in, err)
		}
	}
}
