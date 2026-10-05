package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// uploadSealed uploads a browser-sealed payload over the real API and
// returns its share link.
//
// Parameters:
//   - t: the test.
//   - api: API client for the server.
//   - origin: server origin for the link.
//   - sealed: the bridge's seal reply.
//
// Returns the share link.
func uploadSealed(t *testing.T, api *client.Client, origin string, sealed jsMsg) string {
	t.Helper()
	res, err := api.Create(context.Background(), client.CreateRequest{
		Version: sealed.Version, Nonce: sealed.Nonce, TTL: time.Hour, Body: sealed.Body,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.RawURLEncoding.DecodeString(sealed.Key)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := envelope.NewFragment(sealed.Version, key)
	if err != nil {
		t.Fatal(err)
	}
	link, err := envelope.NewLink(origin, res.ID, frag)
	if err != nil {
		t.Fatal(err)
	}
	return link.String()
}

// assertFilesOnDisk checks that each file was saved in dir with its data.
//
// Parameters:
//   - t: the test.
//   - dir: output directory.
//   - files: expected files.
func assertFilesOnDisk(t *testing.T, dir string, files []jsFile) {
	t.Helper()
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f.Name)) // #nosec G304 -- test temp dir
		if err != nil || !bytes.Equal(data, f.Data) {
			t.Fatalf("%s: %q %v", f.Name, data, err)
		}
	}
}

// assertPayload compares a browser-opened payload with the case sent.
//
// Parameters:
//   - t: the test.
//   - got: the bridge's open reply.
//   - want: the case that was sent.
func assertPayload(t *testing.T, got jsMsg, want interopCase) {
	t.Helper()
	if got.Message != want.message || len(got.Files) != len(want.files) {
		t.Fatalf("got %q with %d files", got.Message, len(got.Files))
	}
	for i, f := range want.files {
		if got.Files[i].Name != f.Name || !bytes.Equal(got.Files[i].Data, f.Data) {
			t.Fatalf("file %d: %q %q", i, got.Files[i].Name, got.Files[i].Data)
		}
	}
}
