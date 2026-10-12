//go:build windows

package reqdb

// checkDir only checks that dir is a directory: Windows protects the
// user's profile with ACLs, not Unix permission bits.
//
// Parameters:
//   - dir: the config directory.
//
// Returns a stat error or nil.
func checkDir(dir string) error {
	_, err := statDir(dir)
	return err
}

// checkFile does nothing on Windows; see checkDir.
//
// Parameters:
//   - path: the request file.
//
// Returns nil.
func checkFile(string) error { return nil }
