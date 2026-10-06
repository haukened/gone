package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestVersion(t *testing.T) {
	cases := []struct {
		name    string
		stamped string
		module  string
		ok      bool
		want    string
	}{
		{"stamped wins", "v3.5.2", "v3.5.3", true, "v3.5.2"},
		{"go install", "dev", "v3.5.3", true, "v3.5.3"},
		{"empty stamp", "", "v3.5.3", true, "v3.5.3"},
		{"local build", "dev", "(devel)", true, "dev"},
		{"no module version", "dev", "", true, "dev"},
		{"no build info", "dev", "v3.5.3", false, "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := readBuildInfo
			t.Cleanup(func() { readBuildInfo = orig })
			readBuildInfo = func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{Main: debug.Module{Version: tc.module}}, tc.ok
			}
			if got := Version(tc.stamped); got != tc.want {
				t.Fatalf("Version(%q) = %q, want %q", tc.stamped, got, tc.want)
			}
		})
	}
}
