package envelope

import "testing"

// FuzzUnpack exercises Unpack with valid and malformed envelope bytes.
//
// Parameters:
//   - f: the fuzz test handle used to register seeds and the fuzz target.
func FuzzUnpack(f *testing.F) {
	for _, in := range unpackInputs() {
		f.Add(in.b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		fuzzUnpackOne(t, b)
	})
}

// FuzzParseFragment exercises ParseFragment with valid and malformed fragments.
//
// Parameters:
//   - f: the fuzz test handle used to register seeds and the fuzz target.
func FuzzParseFragment(f *testing.F) {
	for _, s := range fragmentInputs() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fuzzParseFragmentOne(t, s)
	})
}

// FuzzSanitizeFileName exercises SanitizeFileName idempotence and safety.
//
// Parameters:
//   - f: the fuzz test handle used to register seeds and the fuzz target.
func FuzzSanitizeFileName(f *testing.F) {
	for _, s := range buildSanitize().Names {
		f.Add(s.In)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fuzzSanitizeFileNameOne(t, s)
	})
}
