package envelope

import (
	"bytes"
	"errors"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

// fuzzParseReplyFragmentOne verifies one ParseReplyFragment input: errors
// are public, and an accepted fragment is canonical and well-formed.
//
// Parameters:
//   - t: the test handle.
//   - s: the candidate fragment.
func fuzzParseReplyFragmentOne(t *testing.T, s string) {
	t.Helper()
	f, err := ParseReplyFragment(s)
	if err != nil {
		checkFragmentFuzzError(t, err)
		return
	}
	again, err := NewReplyFragment(f.PublicKey, f.Fill.String())
	if err != nil || f.String() != s || again.String() != s ||
		len(f.PublicKey) != domain.V3PublicKeySize || f.PublicKey[0] != 0x04 {
		t.Fatalf("accepted non-canonical reply fragment %q (%v)", s, err)
	}
}

// fuzzParseLinkOne verifies one ParseLink input: errors are public, and an
// accepted link rebuilds through NewLink and parses back to itself.
//
// Parameters:
//   - t: the test handle.
//   - s: the candidate link.
func fuzzParseLinkOne(t *testing.T, s string) {
	t.Helper()
	l, err := ParseLink(s)
	if err != nil {
		checkLinkFuzzError(t, err)
		return
	}
	built, err := NewLink(l.Origin, l.ID, l.Fragment)
	if err != nil || built.String() != l.String() {
		t.Fatalf("NewLink(%q) = %q, %v", l.Origin, built.String(), err)
	}
	again, err := ParseLink(l.String())
	if err != nil || again.String() != l.String() || !bytes.Equal(again.Fragment.Key(), l.Fragment.Key()) {
		t.Fatalf("link %q renders as %q, which parses as %q (%v)", s, l.String(), again.String(), err)
	}
}

// checkLinkFuzzError verifies that a link fuzz error is public.
//
// Parameters:
//   - t: the test handle.
//   - err: the error returned by ParseLink.
func checkLinkFuzzError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidLink) {
		checkFragmentFuzzError(t, err)
	}
}
