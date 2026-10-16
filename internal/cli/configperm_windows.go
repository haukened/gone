//go:build windows

package cli

import "os"

// checkConfigFile does nothing on Windows: the user's profile is protected
// by ACLs, not Unix permission bits.
//
// Parameters:
//   - f: the open config file.
//   - path: config file path.
//
// Returns nil.
func checkConfigFile(*os.File, string) error { return nil }
