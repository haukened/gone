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

	"github.com/haukened/gone/v3/internal/app"
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
	guard     blobGuard // keeps Reconcile off blobs whose row is pending
}

// New returns a Store implementation of app.SecretStore.
func New(index Index, blobs BlobStorage, clock app.Clock, inlineMax int64) *Store {
	return &Store{index: index, blobs: blobs, clock: clock, inlineMax: inlineMax}
}

var _ app.SecretStore = (*Store)(nil)

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

// Claim reserves a secret for one client and returns its ciphertext without
// deleting it. Inline payloads are returned from memory; external payloads are
// streamed from blob storage via a plain (non-deleting) reader so the client
// can retry within the claim lease. Deletion happens on Ack or expiry; a row
// found dead here is deleted together with its blob.
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
	return buildClaimed(res, s.sweepDead(id, err))
}

// buildClaimed converts an IndexResult into app.Claimed depending on storage mode.
//
// Parameters:
//   - res: row returned by Index.Claim or RequestIndex.ClaimReply.
//   - err: error returned with it, passed through unchanged.
//
// Returns the claimed secret, err, or an error if an external reader is missing.
func buildClaimed(res *IndexResult, err error) (app.Claimed, error) {
	if err != nil {
		return app.Claimed{}, err
	}
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
	return s.dropBlob(id, external, err)
}

// dropBlob finishes an acknowledgement: on success an external payload's blob
// is deleted best-effort, because Reconcile removes any orphan left by a
// failure here.
//
// Parameters:
//   - id: secret or request identifier.
//   - external: whether the deleted row's payload was external.
//   - err: the index's result.
//
// Returns err unchanged.
func (s *Store) dropBlob(id string, external bool, err error) error {
	if err == nil && external {
		_ = s.blobs.Delete(id) // best-effort; Reconcile cleans orphans
	}
	return err
}

// Status reports a pending secret to the sender holding its manage token. A
// row found dead is deleted together with its blob.
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
	st, err := s.index.Status(ctx, id, manageHash, s.clock.Now())
	return st, s.sweepDead(id, err)
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
		return s.sweepDead(id, err)
	}
	if external {
		_ = s.blobs.Delete(id) // best-effort; Reconcile cleans orphans
	}
	return nil
}
