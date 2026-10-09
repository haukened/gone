package store

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

var _ app.RequestStore = (*Store)(nil)

// errNoRequests reports a Store whose index does not implement RequestIndex.
var errNoRequests = errors.New("store: index does not support requests")

// requests returns the index as a RequestIndex.
//
// Returns the request index, or errNoRequests when the store is
// uninitialized or its index lacks request support.
func (s *Store) requests() (RequestIndex, error) {
	if s == nil || s.index == nil || s.clock == nil {
		return nil, errors.New("store not properly initialized")
	}
	ri, ok := s.index.(RequestIndex)
	if !ok {
		return nil, errNoRequests
	}
	return ri, nil
}

// CreateRequest persists a new open request.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - fillHash: hex SHA-256 of the fill token.
//   - manageHash: hex SHA-256 of the requester's manage token.
//   - ttl: lifetime of the request, and of its reply from the fill.
//   - expiresAt: deadline for a reply.
//
// Returns a storage error, if any.
func (s *Store) CreateRequest(ctx context.Context, id, fillHash, manageHash string, ttl time.Duration, expiresAt time.Time) error {
	ri, err := s.requests()
	if err != nil {
		return err
	}
	return ri.InsertRequest(ctx, NewRequestRow{
		ID: id, FillHash: fillHash, ManageHash: manageHash, TTL: ttl,
		CreatedAt: s.clock.Now(), ExpiresAt: expiresAt,
	})
}

// RequestOpen returns the expiry of an open request whose fill hash matches.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - fillHash: hex SHA-256 of the fill token.
//
// Returns the expiry, app.ErrNotFound, or a storage error.
func (s *Store) RequestOpen(ctx context.Context, id, fillHash string) (time.Time, error) {
	ri, err := s.requests()
	if err != nil {
		return time.Time{}, err
	}
	return ri.RequestOpen(ctx, id, fillHash, s.clock.Now())
}

// Fill stores the single reply to an open request. The request is checked
// first, so a closed request never costs a blob write; the payload is then
// written and the request swapped for the reply atomically. If that swap
// fails, an external payload is removed best-effort (Reconcile sweeps any
// orphan).
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - fillHash: hex SHA-256 of the fill token.
//   - meta: protocol metadata of the reply.
//   - r: ciphertext stream of exactly size bytes.
//   - size: ciphertext length.
//
// Returns the reply's expiry, app.ErrNotFound, or a storage error.
func (s *Store) Fill(ctx context.Context, id, fillHash string, meta app.Meta, r io.Reader, size int64) (time.Time, error) {
	ri, err := s.requests()
	if err != nil {
		return time.Time{}, err
	}
	if err = s.validateSaveRequest(size); err != nil {
		return time.Time{}, err
	}
	if _, err = ri.RequestOpen(ctx, id, fillHash, s.clock.Now()); err != nil {
		return time.Time{}, err
	}
	inline, external, err := s.storePayload(ctx, id, r, size)
	if err != nil {
		return time.Time{}, err
	}
	expires, err := ri.FillRequest(ctx, id, fillHash, s.clock.Now(), NewRow{
		Meta: meta, Inline: inline, External: external, Size: size,
	})
	if err != nil && external {
		_ = s.blobs.Delete(id) // best-effort; Reconcile cleans orphans
	}
	return expires, err
}

// RequestStatus reports a request to the requester holding its manage token.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the manage token.
//
// Returns the status, app.ErrNotFound, or a storage error.
func (s *Store) RequestStatus(ctx context.Context, id, manageHash string) (app.RequestStatus, error) {
	ri, err := s.requests()
	if err != nil {
		return app.RequestStatus{}, err
	}
	return ri.RequestStatus(ctx, id, manageHash, s.clock.Now())
}

// ClaimReply reserves a ready reply for the requester and returns its
// ciphertext without deleting it, exactly like Claim.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the manage token.
//   - claimHash: hex SHA-256 of the claim token.
//   - retry: whether the caller is re-presenting an existing token.
//   - claimedUntil: lease deadline recorded for a fresh claim.
//
// Returns the claimed reply, app.ErrNotFound, or a storage error.
func (s *Store) ClaimReply(ctx context.Context, id, manageHash, claimHash string, retry bool, claimedUntil time.Time) (app.Claimed, error) {
	ri, err := s.requests()
	if err != nil {
		return app.Claimed{}, err
	}
	if s.blobs == nil {
		return app.Claimed{}, errors.New("store not properly initialized")
	}
	res, err := ri.ClaimReply(ctx, id, manageHash, claimHash, retry, s.clock.Now(), claimedUntil, s.blobs.Open)
	if err != nil {
		return app.Claimed{}, err
	}
	return buildClaimed(res)
}

// CancelRequest deletes an open request or an unopened reply. A reply's blob
// is removed best-effort; Reconcile sweeps any orphan.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the manage token.
//
// Returns app.ErrNotFound or a storage error.
func (s *Store) CancelRequest(ctx context.Context, id, manageHash string) error {
	ri, err := s.requests()
	if err != nil {
		return err
	}
	external, err := ri.CancelRequest(ctx, id, manageHash, s.clock.Now())
	if err != nil {
		return err
	}
	if external && s.blobs != nil {
		_ = s.blobs.Delete(id) // best-effort; Reconcile cleans orphans
	}
	return nil
}
