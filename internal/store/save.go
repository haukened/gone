package store

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

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
	if err := s.validateSaveRequest(size); err != nil {
		return err
	}
	createdAt := s.clock.Now()
	inline, external, err := s.storePayload(ctx, id, r, size)
	if err != nil {
		return err
	}
	return s.index.Insert(ctx, NewRow{
		ID: id, Meta: meta, Inline: inline, External: external, Size: size,
		ManageHash: manageHash, CreatedAt: createdAt, ExpiresAt: expiresAt,
	})
}

// validateSaveRequest checks whether the Store can persist a payload of size.
//
// Parameters:
//   - size: ciphertext length.
//
// Returns an initialization, size, or nil error.
func (s *Store) validateSaveRequest(size int64) error {
	if s == nil || s.index == nil || s.clock == nil {
		return errors.New("store not properly initialized")
	}
	if size < 0 {
		return errors.New("size must be non-negative")
	}
	return nil
}

// storePayload writes a payload to inline memory or external blob storage.
//
// Parameters:
//   - ctx: request context, reserved for future storage hooks.
//   - id: secret identifier.
//   - r: ciphertext stream of exactly size bytes.
//   - size: ciphertext length.
//
// Returns inline bytes, whether blob storage was used, and any read/write error.
func (s *Store) storePayload(_ context.Context, id string, r io.Reader, size int64) ([]byte, bool, error) {
	if size > s.inlineMax {
		return nil, true, s.blobs.Write(id, r, size)
	}
	inline := make([]byte, size)
	_, err := io.ReadFull(r, inline)
	return inline, false, err
}
