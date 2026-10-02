package filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBlobStoreOpenDoesNotDeleteOnClose(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 32 hex
	data := []byte("secret-bytes")
	if err := bs.Write(id, io.NopCloser(bytesReader(data)), int64(len(data))); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	rc, err := bs.Open(id)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".blob")); err != nil {
		t.Fatalf("expected file to remain after close, got stat err=%v", err)
	}
}

func TestNewBlobBadRoot(t *testing.T) {
	_, err := New("/path/does/not/exist")
	if err == nil {
		t.Fatalf("expected error for non-existent root")
	}
}

func TestWriteBadSize(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	id := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	data := []byte("short")
	err = bs.Write(id, bytesReader(data), int64(len(data)+10)) // request more than available
	if err == nil {
		t.Fatalf("expected error for short read")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF error, got: %v", err)
	}
	// Ensure no file was created
	if _, err := os.Stat(filepath.Join(dir, id+".blob")); !os.IsNotExist(err) {
		t.Fatalf("expected no blob file created, got: %v", err)
	}
}

func TestDeleteEmptyID(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	err = bs.Delete("")
	if err != nil {
		t.Fatalf("expected no error when deleting empty ID")
	}
}

func TestBlobStoreWriteOpenDelete(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	id := "cccccccccccccccccccccccccccccccc"
	data := []byte("secret-bytes")

	if err := bs.Write(id, io.NopCloser(bytesReader(data)), int64(len(data))); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	// second write with same id should fail (file exists)
	if err := bs.Write(id, bytesReader(data), int64(len(data))); err == nil {
		t.Fatalf("expected error on duplicate write")
	}

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
	if string(got) != string(data) {
		t.Fatalf("data mismatch got=%q want=%q", got, data)
	}
	if _, err := bs.Open(id); err != nil {
		t.Fatalf("expected blob to remain after open+close: %v", err)
	}
	if err := bs.Delete(id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := bs.Open(id); err == nil {
		t.Fatalf("expected error opening deleted blob")
	}
	if err := bs.Delete(id); err == nil {
		t.Fatalf("expected error deleting already-deleted blob")
	}
}

func TestBlobStoreListSkipsRecent(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	id := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	payload := []byte("p")
	if err := bs.Write(id, bytesReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Without waiting, List should likely skip due to freshness guard
	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 fresh ids got %v", ids)
	}
	time.Sleep(1100 * time.Millisecond)
	ids, err = bs.List()
	if err != nil {
		t.Fatalf("List after wait: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("expected id after wait, got %v", ids)
	}
}

func TestBlobStoreNewErrors(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("prep file: %v", err)
	}
	if _, err := New(filePath); err == nil {
		t.Fatalf("expected error for non-directory root")
	}
}

func TestBlobStoreInvalidIDs(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	payload := []byte("x")
	cases := []string{
		"../escape",                         // traversal
		"a/b",                               // separator
		"..",                                // traversal
		"..hidden",                          // contains '..'
		"trick..",                           // contains '..'
		"slash/",                            // trailing slash
		`back\\slash`,                       // windows sep
		"short",                             // too short
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",  // invalid hex chars
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",  // uppercase hex not allowed
		"1234567890abcdef1234567890abcde",   // length 31
		"1234567890abcdef1234567890abcdef0", // length 33
	}
	for _, id := range cases {
		if err := bs.Write(id, bytesReader(payload), int64(len(payload))); err == nil {
			t.Fatalf("expected write error for id=%q", id)
		}
		if _, err := bs.Open(id); err == nil {
			t.Fatalf("expected open error for id=%q", id)
		}
		if err := bs.Delete(id); err == nil {
			t.Fatalf("expected delete error for id=%q", id)
		}
	}
}

// bytesReader returns a simple io.Reader over b without copying.
func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct{ b []byte }

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

func TestListAfterDeletingDirectory(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	err = os.RemoveAll(dir)
	if err != nil {
		t.Fatalf("RemoveAll error: %v", err)
	}
	_, err = bs.List()
	if err == nil {
		t.Fatalf("expected error listing after dir removed")
	}
}

func TestListWithNoBlobs(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	// Create some directories inside the blob root
	if err := os.Mkdir(filepath.Join(dir, "subdir1"), 0o700); err != nil {
		t.Fatalf("mkdir subdir1: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir2"), 0o700); err != nil {
		t.Fatalf("mkdir subdir2: %v", err)
	}
	f, err := os.Create(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 ids when only directories present, got: %v", ids)
	}
}
