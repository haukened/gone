package envelope

import "testing"

// FuzzOpenV2Header exercises the v2 header parser through OpenV2 with a
// fixed link key and nonce, seeded with the aead_v2.json blobs.
//
// Parameters:
//   - f: the fuzz test handle used to register seeds and the fuzz target.
func FuzzOpenV2Header(f *testing.F) {
	for _, b := range v2BlobSeeds(f) {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, blob []byte) {
		fuzzOpenV2HeaderOne(t, blob)
	})
}

// FuzzParseReplyFragment exercises ParseReplyFragment, seeded with the
// fragment vectors.
//
// Parameters:
//   - f: the fuzz test handle used to register seeds and the fuzz target.
func FuzzParseReplyFragment(f *testing.F) {
	for _, s := range fragmentSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fuzzParseReplyFragmentOne(t, s)
	})
}

// FuzzParseLink exercises ParseLink with links built from the fragment
// vectors.
//
// Parameters:
//   - f: the fuzz test handle used to register seeds and the fuzz target.
func FuzzParseLink(f *testing.F) {
	for _, s := range linkSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fuzzParseLinkOne(t, s)
	})
}
