package reqdb

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// lockName is the lock file that serializes changes to FileName.
const lockName = "requests.lock"

// Lock timing: changes take milliseconds, so a lock older than staleLock
// was left by a process that died, and waiting longer than lockWait means
// something is wrong.
var (
	lockWait  = 5 * time.Second
	staleLock = 30 * time.Second
	lockPoll  = 10 * time.Millisecond
)

// ErrLocked reports that another gone process held the request file for
// longer than lockWait.
var ErrLocked = errors.New("the request file is in use by another gone process")

// lock takes the lock file, waiting up to lockWait. A lock older than
// staleLock is removed, by one waiter at a time (see breakStale): its
// process died mid-change.
//
// Parameters:
//   - ctx: context.
//
// Returns the unlock function, or ErrLocked, the context error, or an I/O
// error.
func (d *DB) lock(ctx context.Context) (func(), error) {
	path := filepath.Join(d.dir, lockName)
	deadline := time.Now().Add(lockWait)
	unlock := func() { _ = os.Remove(path) }
	for {
		ok, err := createLock(path)
		if err != nil {
			return nil, err
		}
		if ok {
			return unlock, nil
		}
		if ok, err = waitForLock(ctx, path, deadline); err != nil {
			return nil, err
		}
		if ok {
			return unlock, nil
		}
	}
}

// createLock tries to create the lock file.
//
// Parameters:
//   - path: lock file.
//
// Returns true if this call created it, false if it exists, or an I/O error.
func createLock(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm) // #nosec G304 // nosemgrep
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = f.Close()
	return true, nil
}

// waitForLock clears a stale lock or sleeps briefly before the next try.
//
// Parameters:
//   - ctx: context.
//   - path: lock file.
//   - deadline: when to give up.
//
// Returns true if it broke a stale lock and now holds the lock, ErrLocked
// past the deadline, the context error, or false and nil to retry.
func waitForLock(ctx context.Context, path string, deadline time.Time) (bool, error) {
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
		return breakStale(path), nil
	}
	if time.Now().After(deadline) {
		return false, ErrLocked
	}
	t := time.NewTimer(lockPoll)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-t.C:
		return false, nil
	}
}
