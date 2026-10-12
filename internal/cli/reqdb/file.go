package reqdb

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// file is the encoded layout of FileName.
type file struct {
	Version  int
	Requests []Row
}

// update reads the file under the lock, applies change, and writes the
// result back atomically when it differs.
//
// Parameters:
//   - ctx: context; waiting for the lock stops when it is done.
//   - change: transforms the rows.
//
// Returns an I/O, lock, or format error.
func (d *DB) update(ctx context.Context, change func([]Row) []Row) error {
	unlock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	rows, raw, err := d.read()
	if err != nil {
		return err
	}
	changed := change(rows)
	if raw == nil && len(changed) == 0 {
		return nil // nothing saved yet, and nothing to save
	}
	next, err := encode(changed)
	if err != nil || bytes.Equal(next, raw) {
		return err
	}
	return d.write(next)
}

// path returns the request file's path.
//
// Returns the path.
func (d *DB) path() string { return filepath.Join(d.dir, FileName) }

// read decodes the request file. A missing file is empty. On Unix a file
// that others can read or write, that belongs to another user, or that is a
// symlink is refused before it is opened (*UnsafeError).
//
// Returns the rows, the raw bytes, or an error.
func (d *DB) read() ([]Row, []byte, error) {
	if err := checkFile(d.path()); err != nil {
		return nil, nil, err
	}
	// A fixed file name inside the user's own config directory.
	f, err := os.Open(d.path()) // #nosec G304 // nosemgrep
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, nil, err
	}
	rows, err := decode(raw)
	return rows, raw, err
}

// decode parses the file contents.
//
// Parameters:
//   - raw: file bytes.
//
// Returns the rows or an error.
func decode(raw []byte) ([]Row, error) {
	if len(raw) > maxFileBytes {
		return nil, errors.New("the request file is too large")
	}
	var f file
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&f); err != nil {
		return nil, fmt.Errorf("the request file is damaged: %w", err)
	}
	if f.Version != formatVersion {
		return nil, fmt.Errorf("the request file has format %d; this gone understands %d", f.Version, formatVersion)
	}
	return f.Requests, nil
}

// encode serializes rows.
//
// Parameters:
//   - rows: rows to store.
//
// Returns the bytes or an encoding error.
func encode(rows []Row) ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(file{Version: formatVersion, Requests: rows})
	return buf.Bytes(), err
}

// write replaces the request file atomically: a 0600 temp file in the same
// directory is written and synced, renamed over the old one, and the
// directory is synced so the rename itself survives a crash. A reader sees
// either the old file or the new one, never a partial write.
//
// Parameters:
//   - data: new contents.
//
// Returns an I/O error.
func (d *DB) write(data []byte) error {
	tmp, err := os.CreateTemp(d.dir, ".requests-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	err = errors.Join(tmp.Chmod(filePerm), writeAll(tmp, data), tmp.Sync())
	if err = errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	if err = os.Rename(name, d.path()); err != nil {
		return err
	}
	syncDir(d.dir)
	return nil
}

// syncDir flushes a directory entry change to disk. Best effort: some
// platforms (Windows) cannot open a directory for syncing.
//
// Parameters:
//   - dir: directory.
func syncDir(dir string) {
	if f, err := os.Open(dir); err == nil { // #nosec G304 // nosemgrep
		_ = f.Sync()
		_ = f.Close()
	}
}

// writeAll writes data to w.
//
// Parameters:
//   - w: destination.
//   - data: bytes.
//
// Returns the write error.
func writeAll(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}
