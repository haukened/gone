// Package filesystem provides a BlobStorage implementation backed by the local
// filesystem. It stores large ciphertext payloads as immutable blob files.
package filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/haukened/gone/internal/store"
)

// Ensure BlobStore implements store.BlobStorage
var _ store.BlobStorage = (*BlobStore)(nil)

// BlobStore implements store.BlobStorage using the local filesystem.
// Files are named by the secret ID (with a fixed suffix) to simplify lookup.
type BlobStore struct {
	root string
}

// New returns a filesystem-backed blob store rooted at dir. The directory
// must already exist with secure permissions (0700 recommended).
func New(root string) (*BlobStore, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("blob root is not a directory")
	}
	return &BlobStore{root: root}, nil
}

// path constructs the full path to the blob file for a given secret ID.
func (b *BlobStore) path(id string) string { return filepath.Join(b.root, id+".blob") }

// Write stores exactly size bytes from r into a file associated with id.
func (b *BlobStore) Write(id string, r io.Reader, size int64) error {
	p := b.path(id)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.CopyN(f, r, size)
	if err != nil {
		// delete partial file on error
		_ = os.Remove(p)
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	return nil
}

// Open opens a blob file for reading by ID.
func (b *BlobStore) Open(id string) (io.ReadCloser, error) {
	return os.Open(b.path(id)) // #nosec G304 path constructed internally
}

// Delete removes the blob file for a given secret id.
func (b *BlobStore) Delete(id string) error {
	if id == "" {
		return nil
	}
	return os.Remove(b.path(id))
}

// List returns all blob IDs currently present. Higher layers derive orphans
// by diffing against index-reported external IDs.
func (b *BlobStore) List() ([]string, error) {
	entries, err := os.ReadDir(b.root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".blob" {
			continue
		}
		// Basic freshness guard: skip very recent files (<1s) to avoid races.
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < time.Second {
			continue
		}
		ids = append(ids, name[:len(name)-5])
	}
	return ids, nil
}
