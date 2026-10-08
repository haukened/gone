package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenMessageOut checks the --message-out preflight: bad names are
// usage errors, missing directories and existing destinations are I/O
// errors, and success leaves no probe file behind.
//
// Parameters:
//   - t: the test.
func TestOpenMessageOut(t *testing.T) {
	dir := t.TempDir()
	existing := writeTemp(t, "exists", "x")
	tests := []struct {
		path string
		code int
	}{
		{"", exitUsage},
		{dir + string(filepath.Separator), exitUsage},
		{dir + string(filepath.Separator) + ".", exitUsage},
		{dir + string(filepath.Separator) + "..", exitUsage},
		{"-", exitUsage},
		{filepath.Join(dir, "missing", "m"), exitIO},
		{existing, exitIO},
		{dir, exitIO},
		{filepath.Join(dir, strings.Repeat("n", 300)), exitIO},
	}
	for _, tt := range tests {
		if m, err := openMessageOut(tt.path); classify(err).code != tt.code || m != nil {
			t.Errorf("%q: %v", tt.path, err)
		}
	}
	m, err := openMessageOut(filepath.Join(dir, "m"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.close()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("probe left: %v", entries)
	}
	if err := m.write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	checkSavedFile(t, filepath.Join(dir, "m"), "hi")
	m.remove()
	if _, err := os.Stat(filepath.Join(dir, "m")); !os.IsNotExist(err) {
		t.Fatalf("not removed: %v", err)
	}
	var none *messageOut
	none.remove()
	none.close()
}

// TestOpenMessageOutRelative checks a bare file name resolves in the
// working directory.
//
// Parameters:
//   - t: the test.
func TestOpenMessageOutRelative(t *testing.T) {
	t.Chdir(t.TempDir())
	m, err := openMessageOut("m")
	if err != nil {
		t.Fatal(err)
	}
	m.close()
}

// TestOpenMessageOutReadOnly checks that an unwritable directory fails
// before the claim.
//
// Parameters:
//   - t: the test.
func TestOpenMessageOutReadOnly(t *testing.T) {
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil { // #nosec G302 -- test fixture
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) }) // #nosec G302 -- restore for cleanup
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	_, err := openMessageOut(filepath.Join(ro, "m"))
	if classify(err).code != exitIO || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("read-only dir: %v", err)
	}
}
