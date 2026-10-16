package reqdb

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// breakName is the suffix of the file that serializes breaking a stale lock.
const breakName = ".break"

// breakStale removes a stale lock and takes its place, so that when several
// waiters saw the same stale lock only one of them removes it and none
// removes a fresh lock that another waiter created right after.
//
// A bare Stat followed by Remove cannot do that: waiter A removes the stale
// lock and takes a fresh one, and waiter B, whose Stat saw the stale lock,
// then removes A's fresh lock. Instead a waiter must first create the break
// file (requests.lock.break) with O_EXCL; only its creator may act. It looks
// at the lock again, removes it only if it is still stale, takes the lock
// with O_EXCL as usual, and finally removes the break file. A waiter that
// cannot create the break file returns and keeps polling: the lock will
// shortly be free or fresh.
//
// Accepted limitation: a break file older than staleLock is removed by
// whoever notices it (its creator died inside this short section). If two
// waiters both notice it, the second may remove a break file just created by
// a third, so two waiters may be inside the section at once. That needs a
// crash inside the section and then a collision within milliseconds.
//
// Parameters:
//   - path: lock file.
//
// Returns true if the caller now holds the lock.
func breakStale(path string) bool {
	brk := path + breakName
	f, err := os.OpenFile(brk, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm) // #nosec G304 // nosemgrep
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			dropStaleBreak(brk)
		}
		return false
	}
	_ = f.Close()
	defer func() { _ = os.Remove(brk) }()
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
		_ = os.Remove(path)
	}
	ok, _ := createLock(path)
	return ok
}

// dropStaleBreak removes a break file left by a waiter that died inside
// breakStale.
//
// Parameters:
//   - brk: break file.
func dropStaleBreak(brk string) {
	if fi, err := os.Stat(brk); err == nil && time.Since(fi.ModTime()) > staleLock {
		_ = os.Remove(brk)
	}
}
