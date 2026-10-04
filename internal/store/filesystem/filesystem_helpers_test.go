package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// filesystemNewStore creates a blob store rooted at a temporary directory.
//
// Parameters:
//   - t: test handle used for failure reporting.
//
// Returns: the blob store and its root directory.
func filesystemNewStore(t *testing.T) (*BlobStore, string) {
	t.Helper()
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	return bs, dir
}

// filesystemWrite writes payload to id and fails the test on error.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - bs: blob store under test.
//   - id: blob id to write.
//   - payload: bytes to write.
//
// Returns: none; failures abort the test.
func filesystemWrite(t *testing.T, bs *BlobStore, id string, payload []byte) {
	t.Helper()
	if err := bs.Write(id, filesystemBytesReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// filesystemRead reads all bytes for an existing blob id.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - bs: blob store under test.
//   - id: blob id to open and read.
//
// Returns: the bytes read from the blob.
func filesystemRead(t *testing.T, bs *BlobStore, id string) []byte {
	t.Helper()
	rc, err := bs.Open(id)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	return got
}

// filesystemBlobPath returns the blob path for id under dir.
//
// Parameters:
//   - dir: blob root directory.
//   - id: blob id.
//
// Returns: the expected blob path.
func filesystemBlobPath(dir, id string) string { return filepath.Join(dir, id+".blob") }

// filesystemAssertBlobExists verifies that a blob file exists.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//   - id: blob id.
//
// Returns: none; failures abort the test.
func filesystemAssertBlobExists(t *testing.T, dir, id string) {
	t.Helper()
	if _, err := os.Stat(filesystemBlobPath(dir, id)); err != nil {
		t.Fatalf("expected file to remain, got stat err=%v", err)
	}
}

// filesystemAssertOpenFails verifies that a blob cannot be opened.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - bs: blob store under test.
//   - id: blob id.
//
// Returns: none; failures abort the test.
func filesystemAssertOpenFails(t *testing.T, bs *BlobStore, id string) {
	t.Helper()
	if _, err := bs.Open(id); err == nil {
		t.Fatalf("expected error opening blob %q", id)
	}
}

// filesystemBytesReader returns a simple io.Reader over b without copying.
//
// Parameters:
//   - b: bytes to expose through the returned reader.
//
// Returns: an io.Reader over b.
func filesystemBytesReader(b []byte) io.Reader { return &filesystemSliceReader{b: b} }

type filesystemSliceReader struct{ b []byte }

// Read copies bytes from the receiver into p.
//
// Parameters:
//   - p: destination byte slice.
//
// Returns: the number of bytes copied and io.EOF when exhausted.
func (r *filesystemSliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
