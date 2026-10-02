// Package app defines the application layer "ports" (interfaces) and simple
// data contracts that the core use-cases of Gone depend upon. It follows a
// hexagonal (ports & adapters) design: this package declares what the core
// needs, while adapter packages (e.g. SQLite+filesystem storage, HTTP layer,
// janitor jobs) provide concrete implementations. No I/O, logging, SQL, or
// network concerns belong here.
package app

import (
	"context"
	"io"
	"time"
)

// Meta carries minimal per-secret encryption metadata required for clients to
// decrypt the ciphertext. Fields are intentionally small and stable.
type Meta struct {
	Version   uint8  // encryption scheme version negotiated client-side
	NonceB64u string // base64url-encoded nonce provided by the client
}

// Clock abstracts time to enable deterministic testing of TTL / expiry logic.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Claimed describes a secret reserved for delivery by SecretStore.Claim.
type Claimed struct {
	Meta         Meta          // encryption metadata
	Body         io.ReadCloser // ciphertext stream; caller must Close
	Size         int64         // ciphertext length in bytes
	ClaimedUntil time.Time     // lease deadline for retry and acknowledgement
}

// SecretStore is the storage port for secrets. Implementations must provide
// durability and the single-consume invariant. They typically coordinate an
// index (e.g. SQLite) with blob storage (filesystem) but those details are
// outside this interface.
type SecretStore interface {
	// Save persists a new secret blob with metadata and an absolute expiry.
	// 'r' streams exactly 'size' bytes of ciphertext. The call MUST return
	// only after the data and metadata are crash-safe (fsync / committed).
	Save(ctx context.Context, id string, meta Meta, r io.Reader, size int64, expiresAt time.Time) error

	// Claim reserves a secret for a single client and returns its ciphertext
	// without deleting it, enabling retry of an interrupted download.
	// With retry=false the secret must be unclaimed; it is bound to claimHash
	// until claimedUntil. With retry=true claimHash must match the existing
	// claim while its lease is valid. Implementations must guarantee that no
	// other claim hash can ever obtain the secret once claimed, and must return
	// ErrNotFound for absent, expired, or lapsed-claim secrets.
	Claim(ctx context.Context, id, claimHash string, retry bool, claimedUntil time.Time) (Claimed, error)

	// Ack permanently deletes a claimed secret (metadata and payload) once the
	// client confirms full receipt. claimHash must match the active claim;
	// otherwise ErrNotFound is returned.
	Ack(ctx context.Context, id, claimHash string) error

	// DeleteExpired removes (or tombstones) secrets whose expiry is <= t and
	// returns the count of secrets affected. Best-effort cleanup of blob files
	// is acceptable; failures should be surfaced via error.
	DeleteExpired(ctx context.Context, t time.Time) (n int, err error)

	// Reconcile performs consistency checks between metadata/index and blob
	// storage, deleting orphans on either side. It should be idempotent and
	// safe to run periodically.
	Reconcile(ctx context.Context) error
}
