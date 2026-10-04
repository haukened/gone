// Package app contains the application orchestration layer for Gone. It wires
// domain validation with persistence ports without performing any I/O itself.
package app

import (
	"context"
	"errors"
	"fmt"
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

// Created is returned by Service.CreateSecret.
type Created struct {
	ID          domain.SecretID    // new secret identifier
	ExpiresAt   time.Time          // TTL deadline
	ManageToken domain.ManageToken // sender's bearer token for Status and Revoke
}

// Status reports a pending secret to the sender holding its manage token.
//
// Parameters:
//   - ctx: request context.
//   - idStr: secret ID.
//   - tokenStr: manage token issued by CreateSecret.
//
// Returns the secret's timestamps, or domain.ErrInvalidID,
// domain.ErrInvalidManage, ErrNotFound, or a storage error.
func (s *Service) Status(ctx context.Context, idStr, tokenStr string) (SecretStatus, error) {
	tok, err := parseManage(idStr, tokenStr)
	if err != nil {
		return SecretStatus{}, err
	}
	return s.Store.Status(ctx, idStr, tok.Hash())
}

// Revoke permanently deletes a pending secret on behalf of its sender, even if
// a recipient holds an active claim lease.
//
// Parameters:
//   - ctx: request context.
//   - idStr: secret ID.
//   - tokenStr: manage token issued by CreateSecret.
//
// Returns nil on deletion, or domain.ErrInvalidID, domain.ErrInvalidManage,
// ErrNotFound, or a storage error.
func (s *Service) Revoke(ctx context.Context, idStr, tokenStr string) error {
	tok, err := parseManage(idStr, tokenStr)
	if err != nil {
		return err
	}
	if err = s.Store.Revoke(ctx, idStr, tok.Hash()); err != nil {
		return err
	}
	s.inc("secrets_revoked_total")
	return nil
}

// parseManage validates a secret ID and manage token pair.
//
// Parameters:
//   - idStr: secret ID.
//   - tokenStr: manage token.
//
// Returns the parsed token, or domain.ErrInvalidID / domain.ErrInvalidManage.
func parseManage(idStr, tokenStr string) (domain.ManageToken, error) {
	if _, err := domain.ParseID(idStr); err != nil {
		return "", domain.ErrInvalidID
	}
	return domain.ParseManageToken(tokenStr)
}

// inc increments a named counter when metrics are configured. Names are
// hard-coded to avoid importing the metrics package (dependency cycle).
//
// Parameters:
//   - name: counter name.
func (s *Service) inc(name string) {
	if s.Metrics != nil {
		s.Metrics.Inc(name, 1)
	}
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
	s.inc("secrets_consumed_total")
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
func validateTTL(ttl, minTTL, maxTTL time.Duration) error {
	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}
	if minTTL > 0 && ttl < minTTL {
		return fmt.Errorf("ttl below min: %v < %v", ttl, minTTL)
	}
	if maxTTL > 0 && ttl > maxTTL {
		return fmt.Errorf("ttl above max: %v > %v", ttl, maxTTL)
	}
	return nil
}
