// Package store provides the concrete implementation of the application
// SecretStore port by composing lower-layer persistence ports (Index and
// BlobStorage). External packages should construct the store via New and
// interact only through the app.SecretStore interface.
package store

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/haukened/gone/internal/app"
)

// Store composes an Index and BlobStorage to satisfy app.SecretStore.
// It decides whether to inline secret data or place it in blob storage
// based on an inline size threshold.
type Store struct {
	index     Index
	blobs     BlobStorage
	clock     app.Clock
	inlineMax int64
}

// New returns a Store implementation of app.SecretStore.
func New(index Index, blobs BlobStorage, clock app.Clock, inlineMax int64) *Store {
	return &Store{index: index, blobs: blobs, clock: clock, inlineMax: inlineMax}
}

var _ app.SecretStore = (*Store)(nil)

// Save persists a secret. Data <= inlineMax is stored inline; larger data
// is written to blob storage and only the reference is kept in the index.
func (s *Store) Save(ctx context.Context, id string, meta app.Meta, r io.Reader, size int64, expiresAt time.Time) error {
	if s == nil || s.index == nil || s.clock == nil {
		return errors.New("store not properly initialized")
	}
	if size < 0 {
		return errors.New("size must be non-negative")
	}
	createdAt := s.clock.Now()
	var inline []byte
	external := false
	if size <= s.inlineMax {
		// Read fully into memory for inline storage.
		inline = make([]byte, size)
		if _, err := io.ReadFull(r, inline); err != nil {
			return err
		}
	} else {
		if err := s.blobs.Write(id, r, size); err != nil {
			return err
		}
		external = true
	}
	return s.index.Insert(ctx, id, meta, inline, external, size, createdAt, expiresAt)
}

// Consume retrieves a secret exactly once and triggers permanent deletion.
// The index layer hard-deletes the metadata row inside the transaction.
// If the payload was stored in blob storage it is streamed via the blob
// storage's Consume (delete-on-close) reader; inline data is returned via a
// reader. Blob deletion failures during Close are tolerated; reconciliation
// will clean lingering files.
func (s *Store) Consume(ctx context.Context, id string) (meta app.Meta, rc io.ReadCloser, size int64, err error) {
	if s == nil || s.index == nil {
		err = errors.New("store not properly initialized")
		return
	}
	now := s.clock.Now()
	res, cerr := s.index.Consume(ctx, id, now)
	if cerr != nil {
		return meta, nil, 0, cerr
	}
	if expired(now, res.ExpiresAt) {
		return meta, nil, 0, app.ErrNotFound
	}
	return s.buildConsumeResult(id, res)
}

// expired reports whether the resource is expired at now.
func expired(now time.Time, expiresAt time.Time) bool {
	if expiresAt.IsZero() {
		return false
	}
	return now.After(expiresAt) || now.Equal(expiresAt)
}

// buildConsumeResult constructs return values for a consumed secret depending on storage mode.
func (s *Store) buildConsumeResult(id string, res *IndexResult) (meta app.Meta, rc io.ReadCloser, size int64, err error) {
	meta = res.Meta
	size = res.Size
	if res.External {
		f, oErr := s.blobs.Consume(id)
		if oErr != nil {
			return meta, nil, 0, oErr
		}
		return meta, f, size, nil
	}
	rc = io.NopCloser(newInlineReader(res.Inline))
	return meta, rc, int64(len(res.Inline)), nil
}

// DeleteExpired removes expired secrets whose expiry is <= t and returns the count.
// Blob files for expired records are removed best-effort.
func (s *Store) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	expired, err := s.index.DeleteExpired(ctx, t)
	if err != nil {
		return 0, err
	}
	count := len(expired)
	for _, rec := range expired {
		if rec.External {
			_ = s.blobs.Delete(rec.ID) // best-effort
		}
	}
	return count, nil
}

// Reconcile scans for blob orphans and removes them. It can also be extended
// later to verify referential integrity or rebuild indexes.
func (s *Store) Reconcile(ctx context.Context) error {
	if s.index == nil || s.blobs == nil {
		return errors.New("store not properly initialized")
	}
	blobIDs, err := s.blobs.List()
	if err != nil {
		return err
	}
	extIDs, err := s.index.ListExternalIDs(ctx)
	if err != nil {
		return err
	}
	// Build set of index external IDs.
	indexSet := make(map[string]struct{}, len(extIDs))
	for _, id := range extIDs {
		indexSet[id] = struct{}{}
	}
	// Any blob without index entry is orphan.
	for _, bid := range blobIDs {
		if _, ok := indexSet[bid]; !ok {
			_ = s.blobs.Delete(bid)
		}
	}
	return nil
}

// inlineReader provides a zero-allocation Read over a byte slice.
type inlineReader struct {
	b []byte
}

func newInlineReader(b []byte) *inlineReader { return &inlineReader{b: b} }

func (r *inlineReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
