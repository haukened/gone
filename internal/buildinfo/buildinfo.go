// Package buildinfo reports which release a binary was built from.
package buildinfo

import "runtime/debug"

// readBuildInfo is debug.ReadBuildInfo, replaceable in tests.
var readBuildInfo = debug.ReadBuildInfo

// Version returns the release a binary was built from.
//
// Release builds stamp the version with -ldflags "-X main.version=...", and
// that value wins. Builds without it, such as
// "go install github.com/haukened/gone/v3/cmd/gone@v3.5.3", fall back to the
// module version Go records in the binary. Local builds where Go records no
// usable version keep the stamped value ("dev").
//
// Parameters:
//   - stamped: the value of main.version; "dev" or "" when not stamped.
//
// Returns:
//   - string: the release version, e.g. "v3.5.3", or stamped.
func Version(stamped string) string {
	if stamped != "" && stamped != "dev" {
		return stamped
	}
	info, ok := readBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return stamped
	}
	return info.Main.Version
}
