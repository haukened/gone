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

// ConsumeOnce retrieves and deletes (logically) a secret exactly once. If the
// payload was stored in blob storage it streams the data; otherwise it returns
// the inlined bytes. The returned ReadCloser must be fully consumed by caller.
func (s *Store) ConsumeOnce(ctx context.Context, id string) (meta app.Meta, rc io.ReadCloser, size int64, err error) {
	if s == nil || s.index == nil {
		err = errors.New("store not properly initialized")
		return
	}
	now := s.clock.Now()
	meta, inline, external, size, err := s.index.ConsumeOnce(ctx, id, now)
	if err != nil {
		return meta, nil, 0, err
	}
	if external {
		f, oErr := s.blobs.Open(id)
		if oErr != nil {
			return meta, nil, 0, oErr
		}
		return meta, f, size, nil
	}
	// Provide inline bytes via a ReadCloser wrapper.
	rc = io.NopCloser(newInlineReader(inline))
	return meta, rc, int64(len(inline)), nil
}

// ExpireBefore removes expired secrets before the given time and returns the count.
// Blob files for expired records are removed best-effort.
func (s *Store) ExpireBefore(ctx context.Context, t time.Time) (int, error) {
	expired, err := s.index.ExpireBefore(ctx, t)
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
