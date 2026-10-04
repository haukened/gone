package cli

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/haukened/gone/internal/client"
)

const (
	// configDirName is the per-user directory under os.UserConfigDir.
	configDirName = "gone"
	// configFileName is the config file inside configDirName.
	configFileName = "config.json"
	// maxConfigBytes caps how much of the config file is read.
	maxConfigBytes = 64 * 1024
	// configDirPerm is owner-only access (rwx------). Directories need the
	// execute bit to be traversable, so 0o600 would make them unusable.
	configDirPerm = 0o700
)

// config is the persisted CLI configuration. Unknown fields are ignored so
// newer files remain readable.
type config struct {
	Server string `json:"server,omitempty"`
}

// configPath returns the config file location.
//
// Parameters:
//   - env: process environment.
//
// Returns the directory, the file path, or an I/O error.
func configPath(env *Env) (string, string, error) {
	root, err := env.ConfigDir()
	if err != nil {
		return "", "", ioErr("locate config directory", err)
	}
	dir := filepath.Join(root, configDirName)
	return dir, filepath.Join(dir, configFileName), nil
}

// loadConfig reads the config file. A missing file yields an empty config.
// The stored server is re-validated as a bare http(s) origin; whether
// http is acceptable is decided per invocation by --insecure.
//
// Parameters:
//   - env: process environment.
//
// Returns the config or an I/O error.
func loadConfig(env *Env) (config, error) {
	_, path, err := configPath(env)
	if err != nil {
		return config{}, err
	}
	data, err := readConfigFile(path)
	if err != nil || data == nil {
		return config{}, err
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return config{}, ioErr("read config", errors.New(path+" is not valid JSON"))
	}
	if c.Server != "" {
		if c.Server, err = client.NormalizeOrigin(c.Server, true); err != nil {
			return config{}, ioErr("read config", errors.New(path+": invalid server"))
		}
	}
	return c, nil
}

// readConfigFile reads at most maxConfigBytes from path.
//
// Parameters:
//   - path: config file path.
//
// Returns the contents, nil when the file does not exist, or an I/O
// error (including when the file is too large).
func readConfigFile(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- fixed name under the user config dir
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ioErr("read config", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, ioErr("read config", err)
	}
	if len(data) > maxConfigBytes {
		return nil, ioErr("read config", errors.New(path+" is too large"))
	}
	return data, nil
}

// saveConfig atomically writes c with owner-only permissions.
//
// Parameters:
//   - env: process environment.
//   - c: config to persist.
//
// Returns the file path or an I/O error.
func saveConfig(env *Env, c config) (string, error) {
	dir, path, err := configPath(env)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, configDirPerm); err != nil { // nosemgrep: incorrect-default-permission
		return "", ioErr("create config directory", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", ioErr("encode config", err)
	}
	if err := writeFileAtomic(dir, path, append(data, '\n')); err != nil {
		return "", ioErr("write config", err)
	}
	return path, nil
}

// writeFileAtomic writes data to a temporary file in dir, syncs it, and
// renames it over path, so readers never see a partial file.
//
// Parameters:
//   - dir: directory containing path.
//   - path: destination file.
//   - data: file contents.
//
// Returns an error if any step fails; the temporary file is removed.
func writeFileAtomic(dir, path string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
