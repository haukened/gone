package reqdb

import (
	"fmt"
	"io/fs"
	"os"
)

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
