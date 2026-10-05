package envelope

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

var testKey = bytes.Repeat([]byte{0xab}, domain.KeySize)

// TestNewFragmentAcceptedVersions verifies supported fragment versions are accepted.
//
// Parameters:
//   - t: the test handle.
func TestNewFragmentAcceptedVersions(t *testing.T) {
	cases := []uint8{1, 2}
	for _, version := range cases {
		checkNewFragmentVersion(t, version)
	}
}

// checkNewFragmentVersion verifies a fragment version is constructed correctly.
//
// Parameters:
//   - t: the test handle.
//   - version: the fragment version to construct.
func checkNewFragmentVersion(t *testing.T, version uint8) {
	t.Helper()
	f, err := NewFragment(version, testKey)
	if err != nil || f.Version() != version || !bytes.Equal(f.Key(), testKey) {
		t.Fatalf("v%d: %v", version, err)
	}
}

// TestFragmentKeyReturnsCopy verifies Fragment.Key does not expose internals.
//
// Parameters:
//   - t: the test handle.
func TestFragmentKeyReturnsCopy(t *testing.T) {
	f, err := NewFragment(1, testKey)
	if err != nil {
		t.Fatalf("NewFragment: %v", err)
	}
	k := f.Key()
	k[0] = 0
	if f.Key()[0] != 0xab {
		t.Fatal("Key must return a copy")
	}
}

// TestNewFragmentRejectsInvalidInputs verifies invalid versions and keys fail.
//
// Parameters:
//   - t: the test handle.
func TestNewFragmentRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name    string
		version uint8
		key     []byte
		want    error
	}{
		{"v3", 3, testKey, domain.ErrInvalidVersion},
		{"short key", 1, testKey[:31], ErrInvalidKey},
	}
	for _, c := range cases {
		checkNewFragmentRejects(t, c.name, c.version, c.key, c.want)
	}
}

// checkNewFragmentRejects verifies one rejected NewFragment case.
//
// Parameters:
//   - t: the test handle.
//   - name: the case name.
//   - version: the fragment version to construct.
//   - key: the candidate fragment key.
//   - want: the expected error.
func checkNewFragmentRejects(t *testing.T, name string, version uint8, key []byte, want error) {
	t.Helper()
	if _, err := NewFragment(version, key); !errors.Is(err, want) {
		t.Fatalf("%s: %v", name, err)
	}
}

// TestFragmentRoundTrip verifies fragment string rendering and parsing.
//
// Parameters:
//   - t: the test handle.
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

// TestParseFragmentErrors verifies malformed fragments return expected errors.
//
// Parameters:
//   - t: the test handle.
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
		{"v3:" + key, domain.ErrInvalidVersion},
		{"v255:" + key, domain.ErrInvalidVersion},
		{"v2:%%", ErrInvalidFragment},
	}
	for _, c := range cases {
		checkParseFragmentError(t, c.in, c.want)
	}
}

// checkParseFragmentError verifies one rejected ParseFragment case.
//
// Parameters:
//   - t: the test handle.
//   - in: the candidate fragment string.
//   - want: the expected error.
func checkParseFragmentError(t *testing.T, in string, want error) {
	t.Helper()
	if _, err := ParseFragment(in); !errors.Is(err, want) {
		t.Errorf("ParseFragment(%q) = %v, want %v", in, err, want)
	}
}
