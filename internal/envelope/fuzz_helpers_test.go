package envelope

import (
	"bytes"
	"errors"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

// fuzzUnpackOne verifies one Unpack fuzz input.
//
// Parameters:
//   - t: the test handle.
//   - b: the candidate envelope bytes.
func fuzzUnpackOne(t *testing.T, b []byte) {
	t.Helper()
	p, err := Unpack(b)
	if err != nil {
		checkUnpackFuzzError(t, err)
		return
	}
	checkUnpacked(t, b, p)
}

// checkUnpackFuzzError verifies that a fuzz error is an allowed public error.
//
// Parameters:
//   - t: the test handle.
//   - err: the error returned by Unpack.
func checkUnpackFuzzError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidEnvelope) && !errors.Is(err, ErrTooManyFiles) {
		t.Fatalf("unexpected error type: %v", err)
	}
}

// checkUnpacked asserts that a decoded payload is internally consistent.
//
// Parameters:
//   - t: the test handle.
//   - b: the original envelope bytes.
//   - p: the payload returned by Unpack.
func checkUnpacked(t *testing.T, b []byte, p Payload) {
	t.Helper()
	checkFileLimit(t, p)
	n := decodedPayloadSize(t, p)
	if !bytes.HasPrefix(b, Magic[:]) {
		return
	}
	checkMagicPayloadSize(t, n, len(b))
	checkPayloadRoundTrip(t, p)
}

// checkFileLimit verifies that Unpack never accepts more than MaxFiles files.
//
// Parameters:
//   - t: the test handle.
//   - p: the decoded payload.
func checkFileLimit(t *testing.T, p Payload) {
	t.Helper()
	if len(p.Files) > MaxFiles {
		t.Fatal("too many files accepted")
	}
}

// decodedPayloadSize computes the decoded data size while checking metadata.
//
// Parameters:
//   - t: the test handle.
//   - p: the decoded payload.
//
// Returns:
//   - int: the total length of the message and file data.
func decodedPayloadSize(t *testing.T, p Payload) int {
	t.Helper()
	n := len(p.Message)
	for _, f := range p.Files {
		n += len(f.Data)
		checkDecodedFileMetadata(t, f)
	}
	return n
}

// checkDecodedFileMetadata verifies sanitized file metadata.
//
// Parameters:
//   - t: the test handle.
//   - f: the decoded file metadata and data.
func checkDecodedFileMetadata(t *testing.T, f File) {
	t.Helper()
	if SanitizeFileName(f.Name) != f.Name || SafeType(f.Type) != f.Type {
		t.Fatalf("unsanitized metadata %q %q", f.Name, f.Type)
	}
}

// checkMagicPayloadSize verifies decoded payload bytes fit in the input.
//
// Parameters:
//   - t: the test handle.
//   - decoded: the decoded payload data length.
//   - input: the input byte length.
func checkMagicPayloadSize(t *testing.T, decoded int, input int) {
	t.Helper()
	if decoded > input {
		t.Fatal("payload larger than input")
	}
}

// checkPayloadRoundTrip verifies Pack and Unpack preserve a decoded payload.
//
// Parameters:
//   - t: the test handle.
//   - p: the payload to repack and decode.
func checkPayloadRoundTrip(t *testing.T, p Payload) {
	t.Helper()
	again, err := Pack(p)
	if err != nil {
		t.Fatalf("repack: %v", err)
	}
	q, err := Unpack(again)
	if err != nil {
		t.Fatalf("round trip decode: %v", err)
	}
	if !samePayloadShape(q, p) {
		t.Fatalf("round trip mismatch: %v", err)
	}
}

// samePayloadShape reports whether two payloads have the same message and file count.
//
// Parameters:
//   - got: the payload decoded from repacked bytes.
//   - want: the original decoded payload.
//
// Returns:
//   - bool: true when the message and file count match.
func samePayloadShape(got Payload, want Payload) bool {
	return bytes.Equal(got.Message, want.Message) && len(got.Files) == len(want.Files)
}

// fuzzParseFragmentOne verifies one ParseFragment fuzz input.
//
// Parameters:
//   - t: the test handle.
//   - s: the candidate fragment string.
func fuzzParseFragmentOne(t *testing.T, s string) {
	t.Helper()
	fr, err := ParseFragment(s)
	if err != nil {
		checkFragmentFuzzError(t, err)
		return
	}
	checkCanonicalFragment(t, s, fr)
}

// checkFragmentFuzzError verifies that a fragment fuzz error is public.
//
// Parameters:
//   - t: the test handle.
//   - err: the error returned by ParseFragment.
func checkFragmentFuzzError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidFragment) && !errors.Is(err, domain.ErrInvalidVersion) {
		t.Fatalf("unexpected error type: %v", err)
	}
}

// checkCanonicalFragment verifies accepted fragments are canonical.
//
// Parameters:
//   - t: the test handle.
//   - s: the original fragment string.
//   - fr: the parsed fragment.
func checkCanonicalFragment(t *testing.T, s string, fr Fragment) {
	t.Helper()
	if fr.String() != s || len(fr.Key()) != domain.KeySize {
		t.Fatalf("accepted non-canonical fragment %q", s)
	}
}

// fuzzSanitizeFileNameOne verifies one SanitizeFileName fuzz input.
//
// Parameters:
//   - t: the test handle.
//   - s: the candidate file name.
func fuzzSanitizeFileNameOne(t *testing.T, s string) {
	t.Helper()
	out := SanitizeFileName(s)
	checkSanitizedNameBasics(t, s, out)
	checkSanitizedNameRunes(t, out)
}
