//go:build windows

package reqdb

import (
	"errors"
	"io/fs"
)

// lockBeingRemoved reports the error Windows gives when creating a file
// that another process has open for deletion (ERROR_ACCESS_DENIED): the
// previous holder is still removing the lock, so it is busy, not refused.
//
// Parameters:
//   - err: the error from creating the lock file.
//
// Returns true for that error.
func lockBeingRemoved(err error) bool { return errors.Is(err, fs.ErrPermission) }
