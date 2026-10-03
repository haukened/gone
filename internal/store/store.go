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
	metrics   app.Metrics
}

// New returns a Store implementation of app.SecretStore.
func New(index Index, blobs BlobStorage, clock app.Clock, inlineMax int64) *Store {
	return &Store{index: index, blobs: blobs, clock: clock, inlineMax: inlineMax}
}

var _ app.SecretStore = (*Store)(nil)

// CounterClaimsExpired counts secrets that were claimed but never
// acknowledged before their lease lapsed.
const CounterClaimsExpired = "secrets_claims_expired_total"

// WithMetrics attaches an optional metrics sink used to count lapsed claims
// during DeleteExpired.
//
// Parameters:
//   - m: metrics collector (nil disables).
//
// Returns the receiver for chaining.
func (s *Store) WithMetrics(m app.Metrics) *Store {
	s.metrics = m
	return s
}

// Save persists a secret. Data <= inlineMax is stored inline; larger data
// is written to blob storage and only the reference is kept in the index.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - meta: protocol metadata.
//   - manageHash: hex SHA-256 of the sender's manage token.
//   - r: ciphertext stream of exactly size bytes.
//   - size: ciphertext length.
//   - expiresAt: TTL deadline.
//
// Returns an error if the store is uninitialized, size is negative, or a
// write fails.
func (s *Store) Save(ctx context.Context, id string, meta app.Meta, manageHash string, r io.Reader, size int64, expiresAt time.Time) error {
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
	return s.index.Insert(ctx, NewRow{
		ID: id, Meta: meta, Inline: inline, External: external, Size: size,
		ManageHash: manageHash, CreatedAt: createdAt, ExpiresAt: expiresAt,
	})
}

// Claim reserves a secret for one client and returns its ciphertext without
// deleting it. Inline payloads are returned from memory; external payloads are
// streamed from blob storage via a plain (non-deleting) reader so the client
// can retry within the claim lease. Deletion happens on Ack or expiry.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - claimHash: hex SHA-256 of the claim token.
//   - retry: whether the caller is re-presenting an existing token.
//   - claimedUntil: lease deadline recorded for a fresh claim.
//
// Returns the claimed secret or app.ErrNotFound / a storage error.
func (s *Store) Claim(ctx context.Context, id, claimHash string, retry bool, claimedUntil time.Time) (app.Claimed, error) {
	if s == nil || s.index == nil || s.blobs == nil || s.clock == nil {
		return app.Claimed{}, errors.New("store not properly initialized")
	}
	res, err := s.index.Claim(ctx, id, claimHash, retry, s.clock.Now(), claimedUntil, s.blobs.Open)
	if err != nil {
		return app.Claimed{}, err
	}
	return buildClaimed(res)
}

// buildClaimed converts an IndexResult into app.Claimed depending on storage mode.
//
// Parameters:
//   - res: row returned by Index.Claim.
//
// Returns the claimed secret or an error if an external reader is missing.
func buildClaimed(res *IndexResult) (app.Claimed, error) {
	c := app.Claimed{Meta: res.Meta, Size: res.Size, ClaimedUntil: res.ClaimedUntil}
	if res.External {
		if res.Reader == nil {
			return app.Claimed{}, errors.New("external reader missing")
		}
		c.Body = res.Reader
		return c, nil
	}
	c.Body = io.NopCloser(newInlineReader(res.Inline))
	c.Size = int64(len(res.Inline))
	return c, nil
}

// Ack deletes a claimed secret after the client confirms receipt. The index
// row is removed first; blob deletion is best-effort because Reconcile removes
// any orphan left by a failure here.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - claimHash: hex SHA-256 of the claim token.
//
// Returns app.ErrNotFound if no matching claim exists, or a storage error.
func (s *Store) Ack(ctx context.Context, id, claimHash string) error {
	if s == nil || s.index == nil || s.blobs == nil {
		return errors.New("store not properly initialized")
	}
	external, err := s.index.Ack(ctx, id, claimHash)
	if err != nil {
		return err
	}
	if external {
		_ = s.blobs.Delete(id) // best-effort; Reconcile cleans orphans
	}
	return nil
}

// Status reports a pending secret to the sender holding its manage token.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - manageHash: hex SHA-256 of the manage token.
//
// Returns the secret's timestamps, app.ErrNotFound, or a storage error.
func (s *Store) Status(ctx context.Context, id, manageHash string) (app.SecretStatus, error) {
	if s == nil || s.index == nil || s.clock == nil {
		return app.SecretStatus{}, errors.New("store not properly initialized")
	}
	return s.index.Status(ctx, id, manageHash, s.clock.Now())
}

// Revoke deletes a pending secret on behalf of its sender. The index row is
// removed first; blob deletion is best-effort because Reconcile removes any
// orphan left by a failure here.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - manageHash: hex SHA-256 of the manage token.
//
// Returns app.ErrNotFound if no matching pending secret exists, or a storage
// error.
func (s *Store) Revoke(ctx context.Context, id, manageHash string) error {
	if s == nil || s.index == nil || s.blobs == nil || s.clock == nil {
		return errors.New("store not properly initialized")
	}
	external, err := s.index.Revoke(ctx, id, manageHash, s.clock.Now())
	if err != nil {
		return err
	}
	if external {
		_ = s.blobs.Delete(id) // best-effort; Reconcile cleans orphans
	}
	return nil
}

// DeleteExpired removes secrets whose expiry is <= t or whose claim lease has
// lapsed without acknowledgement, and returns the count.
// Blob files for expired records are removed best-effort.
func (s *Store) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	expired, err := s.index.DeleteExpired(ctx, t)
	if err != nil {
		return 0, err
	}
	claimed := 0
	for _, rec := range expired {
		if rec.External {
			_ = s.blobs.Delete(rec.ID) // best-effort
		}
		if rec.Claimed {
			claimed++
		}
	}
	if s.metrics != nil && claimed > 0 {
		s.metrics.Inc(CounterClaimsExpired, int64(claimed))
	}
	return len(expired), nil
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
