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
	// Consume returns secret data and hard-deletes the row in the same transaction.
	Consume(ctx context.Context, id string, now time.Time) (*IndexResult, error)
	DeleteExpired(ctx context.Context, t time.Time) (expired []ExpiredRecord, err error)
	// ListExternalIDs returns IDs of secrets whose payloads are stored externally.
	ListExternalIDs(ctx context.Context) ([]string, error)
}

// IndexResult bundles the data returned by Index.Consume
type IndexResult struct {
	Meta      app.Meta
	Inline    []byte
	External  bool
	Size      int64
	ExpiresAt time.Time
}

// BlobStorage abstracts large payload persistence (e.g. filesystem). Implementations
// MUST provide delete-on-close semantics for Open: calling Open(id) returns an
// io.ReadCloser whose Close method removes (or permanently invalidates) the
// underlying blob file. This enforces one-time consumption symmetry with the
// metadata index hard-delete. Deletion is best-effort; reconciliation routines
// use Delete/List to clean orphans left by crashes occurring after index
// removal but before successful blob deletion.
type BlobStorage interface {
	Write(id string, r io.Reader, size int64) error
	// Consume returns a reader for the blob. Close MUST attempt to delete the
	// blob file. If deletion fails, Close should return that error (unless a
	// prior read error is more relevant). Callers should treat the blob as
	// consumed regardless; janitorial cleanup will retry failed deletions.
	Consume(id string) (io.ReadCloser, error)
	// Delete force-removes a blob by id (used by expiry and reconciliation).
	Delete(id string) error
	// List returns all blob IDs present in storage (filenames sans extension).
	List() ([]string, error)
}

// ExpiredRecord represents an expired secret needing blob cleanup (if blobPath non-empty).
type ExpiredRecord struct {
	ID       string
	External bool // true if payload stored in blob storage
}
