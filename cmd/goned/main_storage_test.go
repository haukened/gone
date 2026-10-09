package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// TestEnsureDataDir verifies directory and blob subdirectory creation.
//
// Parameters:
//   - t: the test handle.
func TestEnsureDataDir(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data-root")
	gotData, gotBlob, err := ensureDataDir(data)
	if err != nil {
		t.Fatalf("ensureDataDir error: %v", err)
	}
	if gotData != data {
		t.Fatalf("data dir mismatch got %s want %s", gotData, data)
	}
	if gotBlob != filepath.Join(data, "blobs") {
		t.Fatalf("blob dir mismatch got %s", gotBlob)
	}
	if _, err := os.Stat(gotData); err != nil {
		t.Fatalf("data dir stat: %v", err)
	}
	if _, err := os.Stat(gotBlob); err != nil {
		t.Fatalf("blob dir stat: %v", err)
	}
}

// TestEnsureDataDir_FilePathError covers a data path that exists as a file.
//
// Parameters:
//   - t: the test handle.
func TestEnsureDataDir_FilePathError(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := ensureDataDir(filePath); err == nil {
		t.Fatalf("expected error for file path")
	}
}

// TestEnsureDataDir_EnforcesPrivatePerms verifies private directory modes.
//
// Parameters:
//   - t: the test handle.
func TestEnsureDataDir_EnforcesPrivatePerms(t *testing.T) {
	for _, tc := range ensurePermCases {
		t.Run(tc.name, func(t *testing.T) {
			assertPrivateDataDirs(t, tc.setup)
		})
	}
}

var ensurePermCases = []struct {
	name  string
	setup func(t *testing.T, dir string)
}{
	{name: "new", setup: func(t *testing.T, _ string) {
		t.Helper()
	}},
	{name: "existing loose data dir", setup: func(t *testing.T, dir string) {
		t.Helper()
		mkdirMode(t, dir, 0o777)
	}},
	{name: "existing loose blobs dir", setup: func(t *testing.T, dir string) {
		t.Helper()
		mkdirMode(t, dir, 0o755)
		mkdirMode(t, filepath.Join(dir, "blobs"), 0o777)
	}},
}

// assertPrivateDataDirs checks the data and blob directory permissions.
//
// Parameters:
//   - t: the test handle.
//   - setup: callback that prepares the data directory before ensureDataDir.
func assertPrivateDataDirs(t *testing.T, setup func(t *testing.T, dir string)) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	setup(t, dir)
	dataDir, blobDir, err := ensureDataDir(dir)
	if err != nil {
		t.Fatalf("ensureDataDir: %v", err)
	}
	for _, p := range []string{dataDir, blobDir} {
		assertPrivateDir(t, p)
	}
}

// assertPrivateDir verifies an existing path has private directory perms.
//
// Parameters:
//   - t: the test handle.
//   - path: the path to inspect.
func assertPrivateDir(t *testing.T, path string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := st.Mode().Perm(); got != privateDirPerm {
		t.Fatalf("%s perm got %o want %o", path, got, privateDirPerm)
	}
}

// mkdirMode creates dir and forces mode regardless of the process umask.
//
// Parameters:
//   - t: the test handle.
//   - dir: the directory to create.
//   - mode: the exact mode to set.
func mkdirMode(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(dir, mode); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
}

// TestEnsureDataDir_BlobsFileError covers a blobs path that exists as a file.
//
// Parameters:
//   - t: the test handle.
func TestEnsureDataDir_BlobsFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blobs"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := ensureDataDir(dir); err == nil {
		t.Fatalf("expected error when blobs is a file")
	}
}

// TestOpenDatabase_Error covers failure with an unwritable directory.
//
// Parameters:
//   - t: the test handle.
func TestOpenDatabase_Error(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, _, err := openDatabase(dir); err == nil {
		t.Fatalf("expected openDatabase error")
	}
}

// TestOpenDatabase_AppliesHardenedPragmas verifies hardened DSN pragmas.
//
// Parameters:
//   - t: the test handle.
func TestOpenDatabase_AppliesHardenedPragmas(t *testing.T) {
	db, _, err := openDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tt := range pragmaCases {
		t.Run(tt.pragma, func(t *testing.T) {
			assertPragma(t, db, tt.pragma, tt.want)
		})
	}
}

var pragmaCases = []struct {
	pragma string
	want   string
}{
	{pragma: "journal_mode", want: "wal"},
	{pragma: "foreign_keys", want: "1"},
	{pragma: "busy_timeout", want: "5000"},
	{pragma: "synchronous", want: "2"},
}

// assertPragma verifies a SQLite pragma value.
//
// Parameters:
//   - t: the test handle.
//   - db: the database to query.
//   - pragma: the pragma name.
//   - want: the expected value.
func assertPragma(t *testing.T, db pragmaQuerier, pragma, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if !strings.EqualFold(got, want) {
		t.Fatalf("PRAGMA %s = %q, want %q", pragma, got, want)
	}
}

type pragmaQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// TestLoadTemplatesFrom_Error covers missing partials or page templates.
//
// Parameters:
//   - t: the test handle.
func TestLoadTemplatesFrom_Error(t *testing.T) {
	if _, err := loadTemplatesFrom(fstest.MapFS{}, "v0.0.0-test", testBundle(t)); err == nil {
		t.Fatalf("expected error due to missing partials template")
	}
}
