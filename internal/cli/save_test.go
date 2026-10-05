package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// openRoot opens dir as an os.Root closed at test end.
//
// Parameters:
//   - t: the test.
//   - dir: directory to open.
//
// Returns the root.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// TestCollisionName checks numbered suffixes for names with and without
// extensions, including dot-files.
//
// Parameters:
//   - t: the test.
func TestCollisionName(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{"a.txt", 0, "a.txt"},
		{"a.txt", 1, "a (1).txt"},
		{"a.tar.gz", 2, "a.tar (2).gz"},
		{"README", 3, "README (3)"},
		{".env", 1, ".env (1)"},
		{"x", 999, "x (999)"},
	}
	for _, tt := range tests {
		if got := collisionName(tt.name, tt.n); got != tt.want {
			t.Errorf("collisionName(%q, %d) = %q, want %q", tt.name, tt.n, got, tt.want)
		}
	}
}

// TestOpenOutDirErrors checks missing and read-only output directories.
//
// Parameters:
//   - t: the test.
func TestOpenOutDirErrors(t *testing.T) {
	if _, err := openOutDir(filepath.Join(t.TempDir(), "missing")); classify(err).code != exitIO {
		t.Fatalf("missing dir: %v", err)
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil { // #nosec G302 -- test fixture
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) }) // #nosec G302 -- restore for cleanup
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	_, err := openOutDir(ro)
	if classify(err).code != exitIO || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("read-only dir: %v", err)
	}
}

// TestOpenOutDirLeavesNoProbe checks the preflight probe is removed.
//
// Parameters:
//   - t: the test.
func TestOpenOutDirLeavesNoProbe(t *testing.T) {
	dir := t.TempDir()
	root, err := openOutDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = root.Close()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("leftover files: %v", entries)
	}
}

// TestSaveFilesCollisionsAndPerms checks numbering, 0600 permissions and
// sanitized names.
//
// Parameters:
//   - t: the test.
func TestSaveFilesCollisionsAndPerms(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	files := []envelope.File{{Name: "a.txt", Data: []byte("1")}, {Name: "a.txt", Data: []byte("22")}, {Name: "CON", Data: nil}}
	saved, err := saveFiles(openRoot(t, dir), dir, files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a (1).txt", "a (2).txt", "_CON"}
	for i, s := range saved {
		if s.Path != filepath.Join(dir, want[i]) || s.Size != len(files[i].Data) || s.Name != files[i].Name {
			t.Fatalf("saved[%d] = %+v", i, s)
		}
		checkSavedFile(t, s.Path, string(files[i].Data))
	}
}

// TestSaveFilesCleansUpOnFailure checks that files written before a failure
// are removed.
//
// Parameters:
//   - t: the test.
func TestSaveFilesCleansUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A name that escapes the root fails inside os.Root.
	files := []envelope.File{{Name: "ok.txt", Data: []byte("x")}, {Name: "../escape", Data: []byte("y")}}
	if _, err := saveFiles(openRoot(t, dir), dir, files); classify(err).code != exitIO {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("leftover files: %v", entries)
	}
}

// TestWriteUniqueExhausted checks the limit on numbered names.
//
// Parameters:
//   - t: the test.
func TestWriteUniqueExhausted(t *testing.T) {
	dir := t.TempDir()
	for n := 0; n <= maxCollisionSuffix; n++ {
		if err := os.WriteFile(filepath.Join(dir, collisionName("f", n)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := writeUnique(openRoot(t, dir), "f", []byte("x"))
	if classify(err).code != exitIO || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("err = %v", err)
	}
}

// TestFinishWriteError checks that a failed write removes the file.
//
// Parameters:
//   - t: the test.
func TestFinishWriteError(t *testing.T) {
	dir := t.TempDir()
	root := openRoot(t, dir)
	f, err := root.OpenFile("x", os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = finishWrite(root, "x", f, []byte("data"))
	if classify(err).code != exitIO {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file not removed: %v", err)
	}
}
