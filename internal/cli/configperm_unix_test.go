//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigPermissionRefusal checks that a config file another user could
// have written is refused with exit 8 and a message naming only the path.
//
// Parameters:
//   - t: the test handle.
func TestConfigPermissionRefusal(t *testing.T) {
	const cfg = `{"server":"https://cfg.example"}`
	tests := []struct {
		name  string
		mode  os.FileMode
		owner func() int
		want  string
	}{
		{"group writable", 0o620, os.Geteuid, "can be written by other users"},
		{"world writable", 0o602, os.Geteuid, "can be written by other users"},
		{"other owner", 0o600, func() int { return os.Geteuid() + 1 }, "belongs to another user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			writeConfig(t, te, cfg)
			path := filepath.Join(te.configDir, configDirName, configFileName)
			if err := os.Chmod(path, tt.mode); err != nil {
				t.Fatal(err)
			}
			configOwner = tt.owner
			t.Cleanup(func() { configOwner = os.Geteuid })
			code := te.run("send", "--message-file", writeTemp(t, "m", "secret words"))
			msg := te.stderr.String()
			if code != exitIO || !strings.Contains(msg, path) || !strings.Contains(msg, tt.want) || strings.Contains(msg, "cfg.example") {
				t.Fatalf("exit %d, stderr %q", code, msg)
			}
		})
	}
}

// TestConfigPermissionAccepted checks that a group-readable (but not
// writable) config is still used.
//
// Parameters:
//   - t: the test handle.
func TestConfigPermissionAccepted(t *testing.T) {
	te := newTestEnv(t)
	writeConfig(t, te, `{"server":"https://cfg.example"}`)
	if err := os.Chmod(filepath.Join(te.configDir, configDirName, configFileName), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := loadConfig(te.Env); err != nil || c.Server != "https://cfg.example" {
		t.Fatalf("config = %+v, %v", c, err)
	}
}

// TestCheckConfigFileStatError checks the fstat failure path.
//
// Parameters:
//   - t: the test handle.
func TestCheckConfigFileStatError(t *testing.T) {
	f, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := checkConfigFile(f, "config.json"); err == nil {
		t.Fatal("stat of a closed file succeeded")
	}
}
