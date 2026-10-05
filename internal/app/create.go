package app

import (
	"context"
	"io"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// CreateSecret validates inputs, assigns a new ID and manage token, determines
// expiry, and persists the secret. Only the SHA-256 of the manage token is
// stored.
//
// Parameters:
//   - ctx: request context for cancellation and deadlines.
//   - ct: ciphertext reader.
//   - size: ciphertext size in bytes.
//   - version: protocol version.
//   - nonce: base64url nonce used for encryption.
//   - ttl: time-to-live for the secret.
//
// Returns the new secret's ID, expiry, and manage token, or
// domain.ErrTTLInvalid, ErrSizeExceeded, domain.ErrInvalidVersion,
// domain.ErrInvalidNonce, or a storage error.
func (s *Service) CreateSecret(ctx context.Context, ct io.Reader, size int64, version uint8, nonce string, ttl time.Duration) (Created, error) {
	if err := s.validateCreateSecretRequest(size, version, nonce, ttl); err != nil {
		return Created{}, err
	}
	id, err := domain.NewID()
	if err != nil { // extremely unlikely, but propagate
		return Created{}, err
	}
	tok, err := domain.NewManageToken()
	if err != nil {
		return Created{}, err
	}
	expiresAt := s.Clock.Now().Add(ttl)
	meta := Meta{Version: version, NonceB64u: nonce}
	if err = s.Store.Save(ctx, id.String(), meta, tok.Hash(), ct, size, expiresAt); err != nil {
		return Created{}, err
	}
	s.inc("secrets_created_total")
	return Created{ID: id, ExpiresAt: expiresAt, ManageToken: tok}, nil
}

// validateCreateSecretRequest checks create-secret inputs in security order.
//
// Parameters:
//   - size: ciphertext size in bytes.
//   - version: protocol version.
//   - nonce: base64url nonce used for encryption.
//   - ttl: requested time-to-live.
//
// Returns the public validation error for the first invalid input.
func (s *Service) validateCreateSecretRequest(size int64, version uint8, nonce string, ttl time.Duration) error {
	if err := validateTTL(ttl, s.MinTTL, s.MaxTTL); err != nil {
		return domain.ErrTTLInvalid
	}
	if size <= 0 || size > s.MaxBytes {
		return ErrSizeExceeded
	}
	return domain.ValidateProtocol(version, nonce)
}
