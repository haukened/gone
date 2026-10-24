//go:build !windows

package reqdb

// lockBeingRemoved is always false here: a removed file is gone at once,
// so creating the lock fails only with ErrExist while it is held.
//
// Parameters:
//   - err: the error from creating the lock file.
//
// Returns false.
func lockBeingRemoved(error) bool { return false }
