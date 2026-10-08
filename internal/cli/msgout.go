package cli

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// messageOut is the --message-out destination. It is opened and checked
// before the claim, so a bad path never consumes the secret.
type messageOut struct {
	root *os.Root
	name string
	path string
}

// openMessageOut checks that path names a file that does not exist yet in
// an existing directory where gone can create files and hard links. It
// runs before the claim.
//
// Parameters:
//   - path: --message-out value.
//
// Returns the opened destination, or a usage or I/O error.
func openMessageOut(path string) (*messageOut, error) {
	dir, name := filepath.Split(path)
	switch name {
	case "", ".", "..", "-":
		return nil, usagef("--message-out must name a file")
	}
	if dir == "" {
		dir = "."
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, ioErr("open message output directory", err)
	}
	m := &messageOut{root: root, name: name, path: path}
	if _, err := root.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
		m.close()
		if err == nil {
			err = fs.ErrExist
		}
		return nil, ioErr("message output file "+path, err)
	}
	probe := ".gone-preflight-" + rand.Text()
	if err := writeNew(root, probe, nil, "message output directory is not writable"); err != nil {
		m.close()
		return nil, err
	}
	_ = root.Remove(probe)
	return m, nil
}

// write saves msg to the destination with mode 0600, never replacing an
// existing file.
//
// Parameters:
//   - msg: message bytes.
//
// Returns nil or an I/O error.
func (m *messageOut) write(msg []byte) error {
	return writeNew(m.root, m.name, msg, "save message")
}

// remove deletes a written message file after a later failure. A nil m
// does nothing.
func (m *messageOut) remove() {
	if m != nil {
		_ = m.root.Remove(m.name)
	}
}

// close releases the destination directory. A nil m does nothing.
func (m *messageOut) close() {
	if m != nil {
		_ = m.root.Close()
	}
}
