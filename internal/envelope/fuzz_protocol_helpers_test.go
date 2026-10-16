package envelope

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

// readVectorFile decodes one test/vectors file into v.
//
// Parameters:
//   - tb: the test handle.
//   - name: file name under vectorDir.
//   - v: destination.
func readVectorFile(tb testing.TB, name string, v any) {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorDir, name)) // #nosec G304 -- fixed test path
	if err != nil {
		tb.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		tb.Fatal(err)
	}
}

// v2BlobSeeds returns every blob in aead_v2.json.
//
// Parameters:
//   - tb: the test handle.
//
// Returns the blobs.
func v2BlobSeeds(tb testing.TB) [][]byte {
	var v aeadV2Vectors
	readVectorFile(tb, "aead_v2.json", &v)
	out := make([][]byte, len(v.Cases))
	for i, c := range v.Cases {
		out[i] = hexBytes(c.Blob)
	}
	return out
}

// fragmentSeeds returns every input of the fragment vectors.
//
// Parameters:
//   - tb: the test handle.
//
// Returns the fragments.
func fragmentSeeds(tb testing.TB) []string {
	var out []string
	for _, name := range []string{"fragment_v1.json", "fragment_v2.json", "fragment_v3.json"} {
		var cases []struct {
			Input string `json:"input"`
		}
		readVectorFile(tb, name, &cases)
		for _, c := range cases {
			out = append(out, c.Input)
		}
	}
	return out
}

// linkSeeds returns secret links built from the fragment vectors, plus a
// few origin and path variants.
//
// Parameters:
//   - tb: the test handle.
//
// Returns the links.
func linkSeeds(tb testing.TB) []string {
	id := strings.Repeat("0123456789abcdef", 2)
	out := []string{
		"http://[::1]:8080/secret/" + id + "#v1:" + strings.Repeat("A", 43),
		"https://user@gone.example/secret/" + id + "#v1:" + strings.Repeat("A", 43),
		"https://gone.example/secret/" + id + "?x#v1:" + strings.Repeat("A", 43),
		"https://gone.example/a/secret/" + id + "#v1:" + strings.Repeat("A", 43),
	}
	for _, frag := range fragmentSeeds(tb) {
		out = append(out, "https://gone.example/secret/"+id+"#"+frag)
	}
	return out
}

// fuzzOpenV2HeaderOne opens blob with an empty passphrase. That is invalid,
// so OpenV2 never runs the KDF: it must report ErrMalformed exactly when
// parseV2Header rejects the header, and ErrInvalidPassphrase otherwise.
//
// Parameters:
//   - t: the test handle.
//   - blob: the candidate v2 blob.
func fuzzOpenV2HeaderOne(t *testing.T, blob []byte) {
	t.Helper()
	hdr, perr := parseV2Header(blob)
	_, err := OpenV2(seq(0, domain.KeySize), "", seq(0, domain.NonceSize), blob)
	want := ErrInvalidPassphrase
	if perr != nil {
		want = ErrMalformed
	}
	if !errors.Is(err, want) || perr != nil && !errors.Is(perr, ErrMalformed) {
		t.Fatalf("OpenV2 = %v, parse = %v, want %v", err, perr, want)
	}
	if perr == nil {
		checkV2Header(t, blob, hdr)
	}
}

// checkV2Header verifies an accepted header against docs/protocol.md §4.2.
//
// Parameters:
//   - t: the test handle.
//   - blob: the blob.
//   - hdr: the parsed header.
func checkV2Header(t *testing.T, blob []byte, hdr v2Header) {
	t.Helper()
	ok := len(blob) >= domain.V2HeaderSize+domain.TagSize &&
		blob[0] == domain.KDFPBKDF2SHA256 &&
		hdr.iterations == binary.BigEndian.Uint32(blob[1:5]) &&
		hdr.iterations >= domain.V2MinIterations && hdr.iterations <= domain.V2MaxIterations &&
		bytes.Equal(hdr.raw, blob[:domain.V2HeaderSize]) && bytes.Equal(hdr.salt, blob[5:domain.V2HeaderSize])
	if !ok {
		t.Fatalf("accepted header %x as %+v", blob[:min(len(blob), domain.V2HeaderSize)], hdr)
	}
}
