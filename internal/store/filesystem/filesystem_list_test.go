package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlobStoreInvalidIDs(t *testing.T) {
	bs, _ := filesystemNewStore(t)
	payload := []byte("x")
	cases := []string{
		"../escape",
		"a/b",
		"..",
		"..hidden",
		"trick..",
		"slash/",
		`back\\slash`,
		"short",
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"1234567890abcdef1234567890abcde",
		"1234567890abcdef1234567890abcdef0",
	}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			filesystemAssertInvalidID(t, bs, id, payload)
		})
	}
}

// filesystemAssertInvalidID verifies all blob operations reject id.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - bs: blob store under test.
//   - id: invalid blob id.
//   - payload: bytes used for write attempts.
//
// Returns: none; failures abort the test.
func filesystemAssertInvalidID(t *testing.T, bs *BlobStore, id string, payload []byte) {
	t.Helper()
	if err := bs.Write(id, filesystemBytesReader(payload), int64(len(payload))); err == nil {
		t.Fatalf("expected write error for id=%q", id)
	}
	if _, err := bs.Open(id); err == nil {
		t.Fatalf("expected open error for id=%q", id)
	}
	if err := bs.Delete(id); err == nil {
		t.Fatalf("expected delete error for id=%q", id)
	}
}

func TestListAfterDeletingDirectory(t *testing.T) {
	bs, dir := filesystemNewStore(t)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll error: %v", err)
	}
	if _, err := bs.List(); err == nil {
		t.Fatalf("expected error listing after dir removed")
	}
}

func TestListWithNoBlobs(t *testing.T) {
	bs, dir := filesystemNewStore(t)
	filesystemCreateNonBlobEntries(t, dir)
	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 ids when only directories present, got: %v", ids)
	}
}

// filesystemCreateNonBlobEntries creates entries List must ignore.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//
// Returns: none; failures abort the test.
func filesystemCreateNonBlobEntries(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"subdir1", "subdir2"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	f, err := os.Create(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}
