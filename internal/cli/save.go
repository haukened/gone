package cli

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/haukened/gone/v3/internal/envelope"
)

// maxCollisionSuffix bounds the "name (n).ext" search.
const maxCollisionSuffix = 999

// savedFile describes one attachment written to disk.
type savedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

// openOutDir opens dir as a root and proves it is writable by creating and
// removing a probe file. It runs before the claim so a bad directory never
// consumes the secret.
//
// Parameters:
//   - dir: output directory.
//
// Returns the opened root or an I/O error.
func openOutDir(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, ioErr("open output directory", err)
	}
	probe := ".gone-preflight-" + rand.Text()
	f, err := root.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_ = f.Close()
		err = root.Remove(probe)
	}
	if err != nil {
		_ = root.Close()
		return nil, ioErr("output directory is not writable", err)
	}
	return root, nil
}

// saveFiles writes every attachment under root without overwriting. On
// failure, files written by this call are removed.
//
// Parameters:
//   - root: output directory root.
//   - dir: output directory path, used for reporting.
//   - files: attachments to write.
//
// Returns the saved files or an I/O error.
func saveFiles(root *os.Root, dir string, files []envelope.File) ([]savedFile, error) {
	saved := make([]savedFile, 0, len(files))
	var names []string
	for _, f := range files {
		name, err := writeUnique(root, saveName(f.Name), f.Data)
		if err != nil {
			for _, n := range names {
				_ = root.Remove(n)
			}
			return nil, err
		}
		names = append(names, name)
		saved = append(saved, savedFile{Name: f.Name, Path: filepath.Join(dir, name), Type: f.Type, Size: len(f.Data)})
	}
	return saved, nil
}

// writeUnique creates name, or the first free "stem (n).ext", and writes
// data to it with mode 0600.
//
// Parameters:
//   - root: output directory root.
//   - name: preferred file name.
//   - data: file contents.
//
// Returns the name used or an I/O error.
func writeUnique(root *os.Root, name string, data []byte) (string, error) {
	for n := 0; n <= maxCollisionSuffix; n++ {
		candidate := collisionName(name, n)
		f, err := root.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", ioErr("save attachment", err)
		}
		return candidate, finishWrite(root, candidate, f, data)
	}
	return "", ioErr("save attachment", fmt.Errorf("too many files named %q", name))
}

// finishWrite writes, syncs and closes f, removing it on failure.
//
// Parameters:
//   - root: output directory root.
//   - name: file name within root.
//   - f: newly created file.
//   - data: file contents.
//
// Returns nil or an I/O error.
func finishWrite(root *os.Root, name string, f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = root.Remove(name)
		return ioErr("save attachment", err)
	}
	return nil
}

// collisionName returns name for n == 0, else "stem (n).ext". A leading dot
// is part of the stem, so ".env" becomes ".env (1)".
//
// Parameters:
//   - name: preferred file name.
//   - n: collision counter.
//
// Returns the candidate name.
func collisionName(name string, n int) string {
	if n == 0 {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem, ext = name, ""
	}
	return fmt.Sprintf("%s (%d)%s", stem, n, ext)
}
