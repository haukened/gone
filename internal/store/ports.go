// Package store defines internal persistence adapter ports used by the
// higher-level SecretStore implementation. These ports isolate the concrete
// SQLite index and filesystem blob storage so they can be tested and evolved
// independently. Callers outside this package interact only with the
// app.SecretStore implementation, not these internal details.
package store

import (
	"context"
	"io"
	"time"

	"github.com/haukened/gone/internal/app"
)

// Index abstracts the metadata/index operations (typically backed by SQLite).
// It stores secret metadata, inlined small ciphertext, and references to blob
// files for larger payloads.
type Index interface {
	Insert(ctx context.Context, id string, meta app.Meta, inline []byte, external bool, size int64, createdAt, expiresAt time.Time) error
	Consume(ctx context.Context, id string, now time.Time) (meta app.Meta, inline []byte, external bool, size int64, err error)
	ExpireBefore(ctx context.Context, t time.Time) (expired []ExpiredRecord, err error)
	// ListExternalIDs returns IDs of secrets whose payloads are stored externally.
	ListExternalIDs(ctx context.Context) ([]string, error)
}

// BlobStorage abstracts large payload persistence on the filesystem.
type BlobStorage interface {
	Write(id string, r io.Reader, size int64) error
	Open(id string) (io.ReadCloser, error)
	Delete(id string) error
	// List returns all blob IDs present in storage (filenames sans extension).
	List() ([]string, error)
}

// ExpiredRecord represents an expired secret needing blob cleanup (if blobPath non-empty).
type ExpiredRecord struct {
	ID       string
	External bool // true if payload stored in blob storage
}
