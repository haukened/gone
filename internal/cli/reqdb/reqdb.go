// Package reqdb keeps the secret requests made by the gone CLI on this
// device, in a file in the user's config directory. Each row holds what only
// the requester may know: the request's manage token and its private key,
// which never appear in a link (docs/protocol.md section 7.5).
//
// The file is gob-encoded: it is internal state for gone, read and written
// only through this package, and not meant to be opened or edited by hand
// ("gone request list" is the way to see it). It is owner-only (0600) in an
// owner-only directory and is rewritten atomically. A lock file serializes
// changes, so two gone processes never lose each other's requests. Like ssh
// with a private key, a file or directory with loose permissions is refused.
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
	"regexp"
	"sort"
	"strings"
	"time"
)

// FileName is the request file inside the CLI config directory.
const FileName = "requests.gob"

// lockName is the lock file that serializes changes to FileName.
const lockName = "requests.lock"

// States a request can be in.
const (
	StateWaiting = "waiting"
	StateReady   = "ready"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
	// minPrefix is the shortest ID prefix Resolve accepts.
	minPrefix = 4
	// formatVersion is the current file layout.
	formatVersion = 1
	// maxFileBytes bounds how much of the file is decoded.
	maxFileBytes = 4 << 20
)

// Lock timing: changes take milliseconds, so a lock older than staleLock
// was left by a process that died, and waiting longer than lockWait means
// something is wrong.
var (
	lockWait  = 5 * time.Second
	staleLock = 30 * time.Second
	lockPoll  = 10 * time.Millisecond
)

// ErrNoMatch reports that no saved request matches an ID or prefix.
var ErrNoMatch = errors.New("no saved request matches")

// ErrBadPrefix reports an ID argument that is not 4 to 32 hex characters.
var ErrBadPrefix = errors.New("a request ID is 4 to 32 lowercase hex characters")

// ErrLocked reports that another gone process held the request file for
// longer than lockWait.
var ErrLocked = errors.New("the request file is in use by another gone process")

// prefixRE accepts 4 to 32 lowercase hex characters.
var prefixRE = regexp.MustCompile(`^[0-9a-f]{4,32}$`)

// AmbiguousError reports a prefix that matches more than one request.
type AmbiguousError struct {
	// IDs are the matching request IDs.
	IDs []string
}

// Error describes the ambiguity.
func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("the ID prefix matches %d requests", len(e.IDs))
}

// Row is one saved request.
type Row struct {
	ID          string    // request ID (32 hex)
	Origin      string    // server origin the request was made on
	Label       string    // the requester's own note; never sent anywhere
	ManageToken string    // authorizes status, the claim, and cancel
	PrivateKey  []byte    // raw P-256 scalar
	ReplyLink   string    // the link to send; it cannot open anything
	State       string    // StateWaiting or StateReady, as last seen
	CreatedAt   time.Time // when the request was made
	ExpiresAt   time.Time // reply deadline, or the reply's own expiry once ready
}

// file is the encoded layout of FileName.
type file struct {
	Version  int
	Requests []Row
}

// DB is the request file in one directory.
type DB struct{ dir string }

// Open prepares dir (owner-only) for the request file. The file itself is
// created on the first change. On Unix a directory that others can write to,
// or that belongs to another user, is refused (*UnsafeError).
//
// Parameters:
//   - dir: the CLI config directory (…/gone).
//
// Returns the handle or an error.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil { // nosemgrep: incorrect-default-permission
		return nil, err
	}
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	return &DB{dir: dir}, nil
}

// Close releases the handle. Every change is already on disk.
//
// Returns nil.
func (d *DB) Close() error { return nil }

// Put saves or replaces a request.
//
// Parameters:
//   - ctx: context.
//   - r: the request.
//
// Returns an I/O error, if any.
func (d *DB) Put(ctx context.Context, r Row) error {
	return d.update(ctx, func(rows []Row) []Row {
		return append(without(rows, r.ID), r)
	})
}

// SetState records the last seen state and expiry of a request.
//
// Parameters:
//   - ctx: context.
//   - id: request ID.
//   - state: StateWaiting or StateReady.
//   - expires: the expiry the server reported.
//
// Returns an I/O error, if any.
func (d *DB) SetState(ctx context.Context, id, state string, expires time.Time) error {
	return d.update(ctx, func(rows []Row) []Row {
		for i := range rows {
			if rows[i].ID == id {
				rows[i].State, rows[i].ExpiresAt = state, expires
			}
		}
		return rows
	})
}

// Delete forgets a request and its key.
//
// Parameters:
//   - ctx: context.
//   - id: request ID.
//
// Returns an I/O error, if any.
func (d *DB) Delete(ctx context.Context, id string) error {
	return d.update(ctx, func(rows []Row) []Row { return without(rows, id) })
}

// List deletes requests past their expiry, then returns the rest newest
// first.
//
// Parameters:
//   - ctx: context.
//   - now: current time.
//
// Returns the live requests or an error.
func (d *DB) List(ctx context.Context, now time.Time) ([]Row, error) {
	var live []Row
	err := d.update(ctx, func(rows []Row) []Row {
		live = live[:0]
		for _, r := range rows {
			if r.ExpiresAt.After(now) {
				live = append(live, r)
			}
		}
		return live
	})
	if err != nil {
		return nil, err
	}
	out := append([]Row(nil), live...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Resolve finds the one live request whose ID starts with prefix.
//
// Parameters:
//   - ctx: context.
//   - prefix: 4 to 32 lowercase hex characters.
//   - now: current time; expired requests are deleted first.
//
// Returns the request, or ErrBadPrefix, ErrNoMatch, *AmbiguousError, or an
// I/O error.
func (d *DB) Resolve(ctx context.Context, prefix string, now time.Time) (Row, error) {
	prefix = strings.ToLower(prefix)
	if len(prefix) < minPrefix || !prefixRE.MatchString(prefix) {
		return Row{}, ErrBadPrefix
	}
	rows, err := d.List(ctx, now)
	if err != nil {
		return Row{}, err
	}
	var hits []Row
	for _, r := range rows {
		if strings.HasPrefix(r.ID, prefix) {
			hits = append(hits, r)
		}
	}
	return pick(hits)
}

// pick returns the single row of hits.
//
// Parameters:
//   - hits: matching rows.
//
// Returns the row, ErrNoMatch, or *AmbiguousError.
func pick(hits []Row) (Row, error) {
	switch len(hits) {
	case 0:
		return Row{}, ErrNoMatch
	case 1:
		return hits[0], nil
	}
	e := &AmbiguousError{}
	for _, h := range hits {
		e.IDs = append(e.IDs, h.ID)
	}
	return Row{}, e
}

// without returns rows minus the one with id.
//
// Parameters:
//   - rows: rows.
//   - id: ID to drop.
//
// Returns the remaining rows.
func without(rows []Row, id string) []Row {
	out := rows[:0]
	for _, r := range rows {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return out
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

// lock takes the lock file, waiting up to lockWait. A lock older than
// staleLock is removed: its process died mid-change.
//
// Parameters:
//   - ctx: context.
//
// Returns the unlock function, or ErrLocked, the context error, or an I/O
// error.
func (d *DB) lock(ctx context.Context) (func(), error) {
	path := filepath.Join(d.dir, lockName)
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm) // #nosec G304 // nosemgrep
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if err = waitForLock(ctx, path, deadline); err != nil {
			return nil, err
		}
	}
}

// waitForLock clears a stale lock or sleeps briefly before the next try.
//
// Parameters:
//   - ctx: context.
//   - path: lock file.
//   - deadline: when to give up.
//
// Returns ErrLocked past the deadline, the context error, or nil to retry.
func waitForLock(ctx context.Context, path string, deadline time.Time) error {
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
		_ = os.Remove(path)
		return nil
	}
	if time.Now().After(deadline) {
		return ErrLocked
	}
	t := time.NewTimer(lockPoll)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
