package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/filesystem"
	"github.com/haukened/gone/internal/store/sqlite"
)

// privateDirPerm is owner-only access (rwx------). Directories need the
// execute (search) bit for the owner to open files inside them, so 0o700 is
// the most restrictive mode that still lets the service operate.
const privateDirPerm os.FileMode = 0o700

// ensureDataDir creates (if needed) the data directory and its blobs
// subdirectory, and enforces owner-only permissions on both, including
// directories that already existed with looser modes.
//
// Parameters:
//   - dir: path to the data directory.
//
// Returns:
//   - string: the data directory path.
//   - string: the blobs directory path.
//   - error: non-nil if a path is not a directory, cannot be created, or its
//     permissions cannot be restricted.
func ensureDataDir(dir string) (string, string, error) {
	if err := ensurePrivateDir(dir); err != nil {
		return "", "", fmt.Errorf("data dir: %w", err)
	}
	blobDir := filepath.Join(dir, "blobs")
	if err := ensurePrivateDir(blobDir); err != nil {
		return "", "", fmt.Errorf("blobs dir: %w", err)
	}
	return dir, blobDir, nil
}

// ensurePrivateDir creates dir if missing and forces its mode to
// privateDirPerm. MkdirAll does not alter pre-existing directories, so the
// mode is always re-applied explicitly.
//
// Parameters:
//   - dir: directory path to create or tighten.
//
// Returns:
//   - error: non-nil if dir exists but is not a directory, or if creation,
//     stat, or chmod fails.
func ensurePrivateDir(dir string) error {
	st, err := os.Stat(dir)
	// Directories need the owner execute (search) bit, so 0o700 is the
	// strictest usable mode; the rules suppressed below assume file semantics.
	switch {
	case errors.Is(err, os.ErrNotExist):
		if mkErr := os.MkdirAll(dir, privateDirPerm); mkErr != nil { // nosemgrep: incorrect-default-permission
			return fmt.Errorf("create: %w", mkErr)
		}
	case err != nil:
		return fmt.Errorf("stat: %w", err)
	case !st.IsDir():
		return fmt.Errorf("not a directory: %s", dir)
	}
	if err := os.Chmod(dir, privateDirPerm); err != nil { // nosemgrep: incorrect-default-permission, go_file-permissions_rule-fileperm
		return fmt.Errorf("restrict permissions: %w", err)
	}
	return nil
}

// openDatabase opens <dataDir>/gone.db with the hardened DSN (WAL, foreign
// keys, busy timeout, FULL synchronous) and initializes the secrets schema.
//
// Parameters:
//   - dataDir: directory holding the SQLite database file.
//
// Returns:
//   - *sql.DB: the opened database handle (caller must Close).
//   - store.Index: SQLite-backed index over db.
//   - error: non-nil if the driver cannot open or the schema cannot be created.
func openDatabase(dataDir string) (*sql.DB, store.Index, error) {
	db, err := sql.Open(sqlite.DriverName, config.SQLiteDSNFor(dataDir))
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite driver: %w", err)
	}
	idx, err := sqlite.New(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("init sqlite schema: %w", err)
	}
	return db, idx, nil
}

// newBlobStorage opens the filesystem blob store rooted at blobDir.
//
// Parameters:
//   - blobDir: existing directory that holds external secret blobs.
//
// Returns:
//   - store.BlobStorage: the filesystem-backed blob store.
//   - error: non-nil if blobDir is missing or not a directory.
func newBlobStorage(blobDir string) (store.BlobStorage, error) {
	blobs, err := filesystem.New(blobDir)
	if err != nil {
		return nil, fmt.Errorf("init blob storage: %w", err)
	}
	return blobs, nil
}
