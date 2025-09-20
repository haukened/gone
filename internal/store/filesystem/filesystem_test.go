package filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeletingReadCloser(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	id := "delete-me"
	data := []byte("secret-bytes")
	if err := bs.Write(id, io.NopCloser(bytesReader(data)), int64(len(data))); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	rc, err := bs.Consume(id)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close(delete) failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".blob")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got stat err=%v", err)
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

	id := "badsize"
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

func TestBlobStoreWriteReadDelete(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	id := "abc123"
	data := []byte("secret-bytes")

	if err := bs.Write(id, io.NopCloser(bytesReader(data)), int64(len(data))); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	// second write with same id should fail (file exists)
	if err := bs.Write(id, bytesReader(data), int64(len(data))); err == nil {
		t.Fatalf("expected error on duplicate write")
	}

	rc, err := bs.Consume(id)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("data mismatch got=%q want=%q", got, data)
	}
	// Close triggers deletion
	if err := rc.Close(); err != nil {
		t.Fatalf("Close(delete) failed: %v", err)
	}
	// File should now be gone; second open should fail.
	if _, err := bs.Consume(id); err == nil {
		t.Fatalf("expected error opening consumed (deleted) blob")
	}

	// After consumption the file is already deleted; Delete should error now.
	if err := bs.Delete(id); err == nil {
		t.Fatalf("expected error deleting already-consumed blob")
	}
}

func TestBlobStoreOpenCloseDeletesWithoutRead(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	id := "noread"
	payload := []byte("x")
	if err := bs.Write(id, bytesReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Write: %v", err)
	}
	rc, err := bs.Consume(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Close without reading should still delete.
	if err := rc.Close(); err != nil {
		t.Fatalf("Close(delete): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".blob")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got stat err=%v", err)
	}
}

func TestBlobStoreListSkipsRecent(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	id := "fresh"
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
	os.Mkdir(filepath.Join(dir, "subdir1"), 0o700)
	os.Mkdir(filepath.Join(dir, "subdir2"), 0o700)
	os.Create(filepath.Join(dir, "file.txt"))

	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 ids when only directories present, got: %v", ids)
	}
}
