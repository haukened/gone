//go:build !windows

package cli

import (
	"errors"
	"os"
	"syscall"
)

// configOwner is the user config.json must belong to; tests replace it.
var configOwner = os.Geteuid

// checkConfigFile refuses a config file that another user could have
// written: one that is group- or world-writable, or owned by someone else.
// The file picks the server every command trusts, so it is not used
// rather than fixed silently. Errors name the path only.
//
// Parameters:
//   - f: the open config file (checked with fstat, so it is the file read).
//   - path: config file path for the message.
//
// Returns an I/O error or nil.
func checkConfigFile(f *os.File, path string) error {
	fi, err := f.Stat()
	if err != nil {
		return ioErr("read config", err)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return ioErr("read config", errors.New(path+" can be written by other users; refusing to use it (fix: chmod 600 "+path+")"))
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != configOwner() {
		return ioErr("read config", errors.New(path+" belongs to another user; refusing to use it"))
	}
	return nil
}
