// Package app contains the application orchestration layer for Gone. It wires
// domain validation with persistence ports without performing any I/O itself.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/haukened/gone/internal/domain"
)

// ErrNotFound indicates the secret was not found or already consumed/expired.
var ErrNotFound = errors.New("secret not found")

// ErrSizeExceeded indicates the provided ciphertext size is zero or exceeds the configured maximum.
var ErrSizeExceeded = errors.New("size exceeded")

// DefaultClaimLease is the claim lease used when Service.ClaimLease is unset.
const DefaultClaimLease = 2 * time.Minute

// Service orchestrates secret creation and one-time consumption using the injected store and clock.
type Service struct {
	Store      SecretStore
	Clock      Clock
	MaxBytes   int64
	MinTTL     time.Duration
	MaxTTL     time.Duration
	ClaimLease time.Duration // window to retry and acknowledge a claim; DefaultClaimLease if <= 0
	Metrics    Metrics       // optional metrics collector (may be nil)
}

// ClaimResult is returned by Service.Claim.
type ClaimResult struct {
	Claimed
	Token domain.ClaimToken // bearer token authorizing retry and Ack
}

// Metrics defines the minimal counter interface the Service depends on.
// Implemented by the metrics.Manager (Inc only) without importing that package
// here to avoid a dependency cycle.
type Metrics interface {
	Inc(name string, delta int64)
}

// CreateSecret validates inputs, assigns a new ID, determines expiry, and persists the secret.
// Returns the generated ID and its expiration timestamp, or
// domain.ErrTTLInvalid, ErrSizeExceeded, domain.ErrInvalidVersion,
// domain.ErrInvalidNonce, or a storage error.
// ctx - the http request context for cancellation and deadlines
// ct - the ciphertext reader
// size - the size of the ciphertext
// version - the version of the secret
// nonce - the nonce used for encryption
// ttl - the time-to-live for the secret
func (s *Service) CreateSecret(ctx context.Context, ct io.Reader, size int64, version uint8, nonce string, ttl time.Duration) (id domain.SecretID, expiresAt time.Time, err error) {
	if err := validateTTL(ttl, s.MinTTL, s.MaxTTL); err != nil {
		return "", time.Time{}, domain.ErrTTLInvalid
	}
	if size <= 0 || size > s.MaxBytes {
		return "", time.Time{}, ErrSizeExceeded
	}
	if err := domain.ValidateProtocol(version, nonce); err != nil {
		return "", time.Time{}, err
	}
	id, genErr := domain.NewID()
	if genErr != nil { // extremely unlikely, but propagate
		return "", time.Time{}, genErr
	}
	now := s.Clock.Now()
	expiresAt = now.Add(ttl)
	meta := Meta{Version: version, NonceB64u: nonce}
	if err = s.Store.Save(ctx, id.String(), meta, ct, size, expiresAt); err != nil {
		return id, expiresAt, err
	}
	if s.Metrics != nil {
		// Assumes metric name constant defined in metrics package; hard-code string to avoid import.
		s.Metrics.Inc("secrets_created_total", 1)
	}
	return id, expiresAt, nil
}

// Claim validates the ID and reserves the secret for the caller.
//
// When tokenStr is empty a fresh claim is made and a new random token is
// issued. When tokenStr is non-empty it must be a previously issued token for
// this secret; the same ciphertext is returned again so interrupted downloads
// can be retried within the lease. Only the SHA-256 of the token is passed to
// the store.
//
// Parameters:
//   - ctx: request context.
//   - idStr: secret ID (32 lowercase hex characters).
//   - tokenStr: existing claim token for a retry, or "" for a fresh claim.
//
// Returns the claimed ciphertext with its token, or domain.ErrInvalidID,
// domain.ErrInvalidClaim, ErrNotFound, or a storage error.
func (s *Service) Claim(ctx context.Context, idStr, tokenStr string) (ClaimResult, error) {
	if _, err := domain.ParseID(idStr); err != nil {
		return ClaimResult{}, domain.ErrInvalidID
	}
	retry := tokenStr != ""
	var (
		tok domain.ClaimToken
		err error
	)
	if retry {
		tok, err = domain.ParseClaimToken(tokenStr)
	} else {
		tok, err = domain.NewClaimToken()
	}
	if err != nil {
		return ClaimResult{}, err
	}
	until := s.Clock.Now().Add(s.claimLease())
	c, err := s.Store.Claim(ctx, idStr, tok.Hash(), retry, until)
	if err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{Claimed: c, Token: tok}, nil
}

// Ack confirms the client received and decrypted the secret, permanently
// deleting it.
//
// Parameters:
//   - ctx: request context.
//   - idStr: secret ID.
//   - tokenStr: claim token issued by Claim.
//
// Returns nil on deletion, or domain.ErrInvalidID, domain.ErrInvalidClaim,
// ErrNotFound, or a storage error.
func (s *Service) Ack(ctx context.Context, idStr, tokenStr string) error {
	if _, err := domain.ParseID(idStr); err != nil {
		return domain.ErrInvalidID
	}
	tok, err := domain.ParseClaimToken(tokenStr)
	if err != nil {
		return err
	}
	if err = s.Store.Ack(ctx, idStr, tok.Hash()); err != nil {
		return err
	}
	if s.Metrics != nil {
		s.Metrics.Inc("secrets_consumed_total", 1)
	}
	return nil
}

// claimLease returns the configured lease or DefaultClaimLease when unset.
func (s *Service) claimLease() time.Duration {
	if s.ClaimLease <= 0 {
		return DefaultClaimLease
	}
	return s.ClaimLease
}

// validateTTL ensures the provided ttl falls within the inclusive [min,max] range.
// Returns an error if out of bounds or zero.
func validateTTL(ttl, min, max time.Duration) error {
	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}
	if min > 0 && ttl < min {
		return fmt.Errorf("ttl below min: %v < %v", ttl, min)
	}
	if max > 0 && ttl > max {
		return fmt.Errorf("ttl above max: %v > %v", ttl, max)
	}
	return nil
}
