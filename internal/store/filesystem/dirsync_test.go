package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncDir(t *testing.T) {
	dir := t.TempDir()
	if err := syncDir(dir); err != nil {
		t.Fatalf("syncDir: %v", err)
	}
	if err := syncDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatalf("syncDir of a missing directory succeeded")
	}
}

func TestWriteRemovesBlobWhenDirSyncFails(t *testing.T) {
	dir := t.TempDir()
	bs, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	errSync := errors.New("sync failed")
	syncBlobDir = func(string) error { return errSync }
	t.Cleanup(func() { syncBlobDir = syncDir })
	id := strings.Repeat("a", 32)
	if err = bs.Write(id, strings.NewReader("data"), 4); !errors.Is(err, errSync) {
		t.Fatalf("Write err = %v, want %v", err, errSync)
	}
	if _, err = os.Stat(filepath.Join(dir, id+".blob")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blob left behind: %v", err)
	}
}
