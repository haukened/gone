//go:build !windows

package reqdb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// getuid is the effective user ID files must belong to; tests replace it.
var getuid = os.Geteuid

// checkDir refuses a directory that others can write to (they could replace
// the request file) or that belongs to another user.
//
// Parameters:
//   - dir: the config directory.
//
// Returns *UnsafeError, a stat error, or nil.
func checkDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if err := checkOwner(dir, fi); err != nil {
		return err
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return &UnsafeError{Path: dir, Reason: fmt.Sprintf("can be written by other users (mode %04o)", fi.Mode().Perm()), Fix: "chmod 700 " + dir}
	}
	return nil
}

// checkFile refuses a request file that others can read or write, that
// belongs to another user, or that is a symlink or not a regular file. A
// missing file is fine.
//
// Parameters:
//   - path: the request file.
//
// Returns *UnsafeError, a stat error, or nil.
func checkFile(path string) error {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return &UnsafeError{Path: path, Reason: "is not a regular file (a symlink or something else)", Fix: ""}
	}
	if err := checkOwner(path, fi); err != nil {
		return err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return &UnsafeError{Path: path, Reason: fmt.Sprintf("can be read or written by other users (mode %04o)", fi.Mode().Perm()), Fix: "chmod 600 " + path}
	}
	return nil
}

// checkOwner refuses a file or directory owned by another user.
//
// Parameters:
//   - path: what was checked.
//   - fi: its file info.
//
// Returns *UnsafeError or nil.
func checkOwner(path string, fi fs.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) == getuid() {
		return nil
	}
	return &UnsafeError{Path: path, Reason: fmt.Sprintf("belongs to another user (uid %d)", st.Uid)}
}
