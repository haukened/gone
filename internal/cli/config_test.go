package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes raw config content for te.
//
// Parameters:
//   - t: the test handle.
//   - te: the test environment.
//   - data: file content.
func writeConfig(t *testing.T, te *testEnv, data string) {
	t.Helper()
	dir := filepath.Join(te.configDir, configDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLoadConfigErrors checks malformed, oversized and unreadable configs.
//
// Parameters:
//   - t: the test handle.
func TestLoadConfigErrors(t *testing.T) {
	tests := map[string]string{
		"invalid json":   "{",
		"invalid server": `{"server":"ftp://x"}`,
		"too large":      strings.Repeat(" ", maxConfigBytes+1),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			te := newTestEnv(t)
			writeConfig(t, te, data)
			if _, err := loadConfig(te.Env); classify(err).code != exitIO {
				t.Fatalf("err = %v", err)
			}
		})
	}
	te := newTestEnv(t)
	if err := os.MkdirAll(filepath.Join(te.configDir, configDirName, configFileName), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(te.Env); classify(err).code != exitIO {
		t.Fatalf("directory as config err = %v", err)
	}
	te.ConfigDir = func() (string, error) { return "", errors.New("no home") }
	if _, err := loadConfig(te.Env); classify(err).code != exitIO {
		t.Fatalf("ConfigDir err = %v", err)
	}
	if _, err := saveConfig(te.Env, config{}); classify(err).code != exitIO {
		t.Fatalf("save ConfigDir err = %v", err)
	}
}

// TestLoadConfigConfinement checks that the config file cannot be read
// through a symlink escaping the config directory, and that a config
// directory path that is not a directory is reported as an I/O error.
//
// Parameters:
//   - t: the test handle.
func TestLoadConfigConfinement(t *testing.T) {
	te := newTestEnv(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"server":"https://evil.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(te.configDir, configDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, configFileName)); err != nil {
		t.Fatal(err)
	}
	if c, err := loadConfig(te.Env); classify(err).code != exitIO || c.Server != "" {
		t.Fatalf("symlink escape: c = %+v, err = %v", c, err)
	}
	notDir := newTestEnv(t)
	if err := os.WriteFile(filepath.Join(notDir.configDir, configDirName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(notDir.Env); classify(err).code != exitIO {
		t.Fatalf("config dir is a file: err = %v", err)
	}
}

// TestSaveConfig checks the saved file round-trips with private
// permissions.
//
// Parameters:
//   - t: the test handle.
func TestSaveConfig(t *testing.T) {
	te := newTestEnv(t)
	path, err := saveConfig(te.Env, config{Server: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v, %v", info, err)
	}
	c, err := loadConfig(te.Env)
	if err != nil || c.Server != "https://example.com" {
		t.Fatalf("load = %+v, %v", c, err)
	}
}

// TestSaveConfigMkdirFails checks that an unusable config directory is an
// I/O error.
//
// Parameters:
//   - t: the test handle.
func TestSaveConfigMkdirFails(t *testing.T) {
	blocked := newTestEnv(t)
	if err := os.WriteFile(filepath.Join(blocked.configDir, configDirName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveConfig(blocked.Env, config{}); classify(err).code != exitIO {
		t.Fatalf("mkdir err = %v", err)
	}
}

// TestWriteFileAtomicErrors checks a missing directory and a failed rename,
// and that no temporary file is left behind.
//
// Parameters:
//   - t: the test handle.
func TestWriteFileAtomicErrors(t *testing.T) {
	if err := writeFileAtomic(filepath.Join(t.TempDir(), "missing"), "x", nil); err == nil {
		t.Fatal("missing dir accepted")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(dir, target, []byte("x")); err == nil {
		t.Fatal("rename over non-empty dir accepted")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".config-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

// TestResolveServer checks flag, env, config and default precedence.
//
// Parameters:
//   - t: the test handle.
func TestResolveServer(t *testing.T) {
	te := newTestEnv(t)
	check := func(flagValue, want, source string) {
		t.Helper()
		got, src, err := resolveServer(te.Env, flagValue)
		if err != nil || got != want || src != source {
			t.Fatalf("resolve = %q %q %v; want %q %q", got, src, err, want, source)
		}
	}
	check("", defaultServer, "default")
	writeConfig(t, te, `{"server":"https://cfg.example"}`)
	check("", "https://cfg.example", "config")
	te.vars[serverEnv] = "https://env.example"
	check("", "https://env.example", serverEnv)
	check("https://flag.example", "https://flag.example", "--server")

	delete(te.vars, serverEnv)
	writeConfig(t, te, "{")
	if _, _, err := resolveServer(te.Env, ""); err == nil {
		t.Fatal("bad config accepted")
	}
}

// TestNewClientErrors checks origin validation messages.
//
// Parameters:
//   - t: the test handle.
func TestNewClientErrors(t *testing.T) {
	te := newTestEnv(t)
	c := common{timeout: 1}
	_, err := newClient(te.Env, "http://x.example", "--server", c)
	if err == nil || !strings.Contains(err.Error(), "--insecure") {
		t.Fatalf("http err = %v", err)
	}
	_, err = newClient(te.Env, "not a url", "--server", c)
	if err == nil || !strings.Contains(err.Error(), "not a valid server origin") {
		t.Fatalf("invalid err = %v", err)
	}
	c.insecure = true
	if _, err := newClient(te.Env, "http://localhost:8080", "--server", c); err != nil {
		t.Fatalf("insecure localhost err = %v", err)
	}
	if _, err := newClient(te.Env, "http://x.example", "--server", c); err == nil {
		t.Fatal("insecure remote http was accepted")
	}
}

// TestRunSet checks the "set server" command.
//
// Parameters:
//   - t: the test handle.
func TestRunSet(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"valid", []string{"set", "server", "https://Example.COM/"}, exitOK},
		{"insecure", []string{"set", "server", "http://localhost:8080", "--insecure"}, exitOK},
		{"http", []string{"set", "server", "http://localhost:8080"}, exitUsage},
		{"remote http", []string{"set", "server", "http://192.168.1.10:8080", "--insecure"}, exitUsage},
		{"invalid", []string{"set", "server", "nope"}, exitUsage},
		{"wrong key", []string{"set", "color", "x"}, exitUsage},
		{"missing", []string{"set", "server"}, exitUsage},
		{"bad flag", []string{"set", "--nope"}, exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			if code := te.run(tt.args...); code != tt.code {
				t.Fatalf("exit %d: %s", code, te.stderr)
			}
		})
	}
	te := newTestEnv(t)
	te.ConfigDir = func() (string, error) { return "", errors.New("no home") }
	if code := te.run("set", "server", "https://x.example"); code != exitIO {
		t.Fatalf("exit %d", code)
	}
}
