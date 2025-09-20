// Package app contains the application orchestration layer for Gone. It wires
// domain validation with persistence ports without performing any I/O itself.
package app

import (
	"context"
	"errors"
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
	if err := domain.ValidateTTL(ttl, s.MinTTL, s.MaxTTL); err != nil {
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
	return id, expiresAt, nil
}

// ConsumeOnce validates the provided ID then delegates to the store for one-time retrieval.
func (s *Service) ConsumeOnce(ctx context.Context, idStr string) (Meta, io.ReadCloser, int64, error) {
	if _, err := domain.ParseID(idStr); err != nil {
		return Meta{}, nil, 0, domain.ErrInvalidID
	}
	meta, rc, size, err := s.Store.ConsumeOnce(ctx, idStr)
	return meta, rc, size, err
}
