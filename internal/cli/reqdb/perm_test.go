package reqdb

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatDir(t *testing.T) {
	dir := t.TempDir()
	if fi, err := statDir(dir); err != nil || !fi.IsDir() {
		t.Fatalf("statDir(dir) = %v, %v", fi, err)
	}
	file := filepath.Join(dir, "plain")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := statDir(file); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("statDir(file) err = %v", err)
	}
	if _, err := statDir(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("statDir(missing) err = %v", err)
	}
}
