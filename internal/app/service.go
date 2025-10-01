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

// Service orchestrates secret creation and one-time consumption using the injected store and clock.
type Service struct {
	Store    SecretStore
	Clock    Clock
	MaxBytes int64
	MinTTL   time.Duration
	MaxTTL   time.Duration
	Metrics  Metrics // optional metrics collector (may be nil)
}

// Metrics defines the minimal counter interface the Service depends on.
// Implemented by the metrics.Manager (Inc only) without importing that package
// here to avoid a dependency cycle.
type Metrics interface {
	Inc(name string, delta int64)
}

// CreateSecret validates inputs, assigns a new ID, determines expiry, and persists the secret.
// Returns the generated ID and its expiration timestamp.
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

// Consume validates the provided ID then delegates to the store for one-time retrieval.
func (s *Service) Consume(ctx context.Context, idStr string) (Meta, io.ReadCloser, int64, error) {
	if _, err := domain.ParseID(idStr); err != nil {
		return Meta{}, nil, 0, domain.ErrInvalidID
	}
	meta, rc, size, err := s.Store.Consume(ctx, idStr)
	if err == nil && s.Metrics != nil {
		s.Metrics.Inc("secrets_consumed_total", 1)
	}
	return meta, rc, size, err
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
