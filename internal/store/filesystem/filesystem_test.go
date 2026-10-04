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
	bs, dir := filesystemNewStore(t)
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	data := []byte("secret-bytes")
	filesystemWrite(t, bs, id, data)
	rc, err := bs.Open(id)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	filesystemAssertBlobExists(t, dir, id)
}

func TestNewBlobBadRoot(t *testing.T) {
	_, err := New("/path/does/not/exist")
	if err == nil {
		t.Fatalf("expected error for non-existent root")
	}
}

func TestWriteBadSize(t *testing.T) {
	bs, dir := filesystemNewStore(t)
	id := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	data := []byte("short")
	err := bs.Write(id, filesystemBytesReader(data), int64(len(data)+10))
	if err == nil {
		t.Fatalf("expected error for short read")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF error, got: %v", err)
	}
	if _, err := os.Stat(filesystemBlobPath(dir, id)); !os.IsNotExist(err) {
		t.Fatalf("expected no blob file created, got: %v", err)
	}
}

func TestDeleteEmptyID(t *testing.T) {
	bs, _ := filesystemNewStore(t)
	if err := bs.Delete(""); err != nil {
		t.Fatalf("expected no error when deleting empty ID")
	}
}

func TestBlobStoreWriteOpenDelete(t *testing.T) {
	bs, _ := filesystemNewStore(t)
	id := "cccccccccccccccccccccccccccccccc"
	data := []byte("secret-bytes")
	filesystemWrite(t, bs, id, data)
	if err := bs.Write(id, filesystemBytesReader(data), int64(len(data))); err == nil {
		t.Fatalf("expected error on duplicate write")
	}
	if got := filesystemRead(t, bs, id); string(got) != string(data) {
		t.Fatalf("data mismatch got=%q want=%q", got, data)
	}
	if _, err := bs.Open(id); err != nil {
		t.Fatalf("expected blob to remain after open+close: %v", err)
	}
	if err := bs.Delete(id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	filesystemAssertOpenFails(t, bs, id)
	if err := bs.Delete(id); err == nil {
		t.Fatalf("expected error deleting already-deleted blob")
	}
}

func TestBlobStoreListSkipsRecent(t *testing.T) {
	bs, _ := filesystemNewStore(t)
	id := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	filesystemWrite(t, bs, id, []byte("p"))
	filesystemAssertListedIDs(t, bs, nil)
	time.Sleep(1100 * time.Millisecond)
	filesystemAssertListedIDs(t, bs, []string{id})
}

// filesystemAssertListedIDs verifies the blob list matches want.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - bs: blob store under test.
//   - want: expected listed ids in order.
//
// Returns: none; failures abort the test.
func filesystemAssertListedIDs(t *testing.T, bs *BlobStore, want []string) {
	t.Helper()
	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != len(want) {
		t.Fatalf("expected %d ids got %v", len(want), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
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
