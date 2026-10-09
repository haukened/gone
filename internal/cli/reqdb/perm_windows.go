//go:build windows

package reqdb

import (
	"fmt"
	"os"
)

// checkDir only checks that dir is a directory: Windows protects the
// user's profile with ACLs, not Unix permission bits.
//
// Parameters:
//   - dir: the config directory.
//
// Returns a stat error or nil.
func checkDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return nil
}

// checkFile does nothing on Windows; see checkDir.
//
// Parameters:
//   - path: the request file.
//
// Returns nil.
func checkFile(string) error { return nil }
