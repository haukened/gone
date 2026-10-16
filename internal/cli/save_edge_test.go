package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// dirNames lists the entry names in dir.
//
// Parameters:
//   - t: the test.
//   - dir: directory to list.
//
// Returns the names.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestSaveFilesLongUnicodeName checks that a 255 code point name whose
// UTF-8 is over 255 bytes is saved under a shortened valid name, twice if
// need be, instead of failing with ENAMETOOLONG.
func TestSaveFilesLongUnicodeName(t *testing.T) {
	dir := t.TempDir()
	name := envelope.SanitizeFileName(strings.Repeat("é", 255))
	if len(name) <= 255 {
		t.Fatalf("test name has only %d bytes", len(name))
	}
	files := []envelope.File{{Name: name, Data: []byte("1")}, {Name: name, Data: []byte("2")}}
	saved, err := saveFiles(openRoot(t, dir), dir, files)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range saved {
		if len(filepath.Base(s.Path)) > 255 {
			t.Errorf("saved[%d] name has %d bytes", i, len(filepath.Base(s.Path)))
		}
		checkSavedFile(t, s.Path, string(files[i].Data))
	}
	if saved[0].Path == saved[1].Path {
		t.Fatal("second file replaced the first")
	}
}

// TestWriteUniqueNameTooLong checks the failure when the OS refuses a name
// (ENAMETOOLONG): a clean I/O error and no file or fragment left behind.
func TestWriteUniqueNameTooLong(t *testing.T) {
	dir := t.TempDir()
	_, err := writeUnique(openRoot(t, dir), strings.Repeat("é", 200)+".txt", []byte("x"))
	if err == nil {
		t.Skip("file system accepts names over 255 bytes")
	}
	if classify(err).code != exitIO {
		t.Fatalf("err = %v", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Fatalf("left behind: %v", names)
	}
}

// TestSaveFilesRollbackKeepsOthersFiles checks that a failed save removes
// only the files this call wrote, never a file that was already there.
func TestSaveFilesRollbackKeepsOthersFiles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "a (1).txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("mine"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := []envelope.File{{Name: "a.txt", Data: []byte("x")}, {Name: "a.txt", Data: []byte("y")}, {Name: "../escape"}}
	if _, err := saveFiles(openRoot(t, dir), dir, files); err == nil {
		t.Fatal("want error")
	}
	got := dirNames(t, dir)
	if len(got) != 2 {
		t.Fatalf("after rollback: %v", got)
	}
	for _, n := range got {
		checkSavedFile(t, filepath.Join(dir, n), "mine")
	}
}

// TestSaveFilesSanitizationCollision checks two names that become equal
// after sanitization are both saved.
func TestSaveFilesSanitizationCollision(t *testing.T) {
	dir := t.TempDir()
	files := []envelope.File{{Name: "a?.txt", Data: []byte("1")}, {Name: "a*.txt", Data: []byte("2")}}
	saved, err := saveFiles(openRoot(t, dir), dir, files)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Path != filepath.Join(dir, "a_.txt") || saved[1].Path != filepath.Join(dir, "a_ (1).txt") {
		t.Fatalf("saved = %+v", saved)
	}
	checkSavedFile(t, saved[0].Path, "1")
	checkSavedFile(t, saved[1].Path, "2")
}

// TestSaveFilesCollisionExhausted checks that when all numbered names are
// taken the error is clean, files written earlier in the call are removed
// and the existing files are untouched.
func TestSaveFilesCollisionExhausted(t *testing.T) {
	dir := t.TempDir()
	fillCollisions(t, dir, "a.txt")
	files := []envelope.File{{Name: "ok.txt", Data: []byte("x")}, {Name: "a.txt", Data: []byte("y")}}
	_, err := saveFiles(openRoot(t, dir), dir, files)
	if classify(err).code != exitIO || !strings.Contains(err.Error(), "too many files") {
		t.Fatalf("err = %v", err)
	}
	if got := dirNames(t, dir); len(got) != maxCollisionSuffix+1 {
		t.Fatalf("%d files, want %d", len(got), maxCollisionSuffix+1)
	}
	if _, err := os.Stat(filepath.Join(dir, "ok.txt")); !os.IsNotExist(err) {
		t.Fatalf("ok.txt not rolled back: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil || info.Size() != 0 {
		t.Fatalf("existing a.txt changed: %v %v", info, err)
	}
}

// TestSaveFilesNeverFollowsSymlinks checks that symlinks in the output
// directory are never followed: one named like the attachment is treated as
// taken (even when dangling) and a link to another directory cannot be
// used to write outside the output directory (os.Root guarantees this).
func TestSaveFilesNeverFollowsSymlinks(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"target.txt":  victim,
		"dangling.md": filepath.Join(outside, "created-by-attacker"),
		"sub":         outside,
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Skipf("cannot create symlinks: %v", err)
		}
	}
	files := []envelope.File{{Name: "target.txt", Data: []byte("new")}, {Name: "dangling.md", Data: []byte("new")}}
	saved, err := saveFiles(openRoot(t, dir), dir, files)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Path != filepath.Join(dir, "target (1).txt") || saved[1].Path != filepath.Join(dir, "dangling (1).md") {
		t.Fatalf("saved = %+v", saved)
	}
	if _, err := saveFiles(openRoot(t, dir), dir, []envelope.File{{Name: "sub/x.txt", Data: []byte("new")}}); err == nil {
		t.Fatal("wrote through a directory symlink")
	}
	checkSavedFile(t, victim, "original")
	if got := dirNames(t, outside); len(got) != 1 {
		t.Fatalf("outside directory changed: %v", got)
	}
}
