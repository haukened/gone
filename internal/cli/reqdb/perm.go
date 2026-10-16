package reqdb

import (
	"fmt"
	"io/fs"
	"os"
)

// Platform note: on Unix, checkDir and checkFile enforce owner-only access
// (see perm_unix.go). On Windows they do not: checkDir only verifies that the
// path is a directory and checkFile accepts everything, because Windows
// protects the user's profile with ACLs rather than permission bits and gone
// does not inspect or set ACLs. On Windows the request file (which holds
// private keys) is therefore only as private as the directory it lives in.

// UnsafeError reports a request file or directory whose permissions or
// owner would let someone else read or replace the keys in it. Like ssh with
// a private key, gone refuses to use it rather than fixing it silently: the
// keys may already have been exposed.
type UnsafeError struct {
	Path   string // the file or directory
	Reason string // what is wrong
	Fix    string // a command that fixes it, or ""
}

// Error describes the problem and how to fix it.
func (e *UnsafeError) Error() string {
	msg := fmt.Sprintf("%s %s; refusing to use it because the keys in it may have been exposed", e.Path, e.Reason)
	if e.Fix != "" {
		msg += ". Fix it with: " + e.Fix
	}
	return msg
}

// statDir stats dir and checks that it is a directory.
//
// Parameters:
//   - dir: the config directory.
//
// Returns its file info, or a stat or not-a-directory error.
func statDir(dir string) (fs.FileInfo, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	return fi, nil
}
