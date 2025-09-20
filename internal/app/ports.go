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

// SecretStore is the storage port for secrets. Implementations must provide
// durability and the single-consume invariant. They typically coordinate an
// index (e.g. SQLite) with blob storage (filesystem) but those details are
// outside this interface.
type SecretStore interface {
	// Save persists a new secret blob with metadata and an absolute expiry.
	// 'r' streams exactly 'size' bytes of ciphertext. The call MUST return
	// only after the data and metadata are crash-safe (fsync / committed).
	Save(ctx context.Context, id string, meta Meta, r io.Reader, size int64, expiresAt time.Time) error

	// Consume atomically marks the secret as consumed (exactly once) and
	// returns its metadata, a reader for the ciphertext, and its size. If the
	// secret is absent, expired, or already consumed, an error is returned.
	// Implementations must guarantee that no concurrent caller can retrieve
	// the same secret after a successful consume.
	Consume(ctx context.Context, id string) (meta Meta, rc io.ReadCloser, size int64, err error)

	// ExpireBefore removes (or tombstones) secrets whose expiry precedes 't'
	// and returns the count of secrets affected. Best-effort cleanup of blob
	// files is acceptable; failures should be surfaced via error.
	ExpireBefore(ctx context.Context, t time.Time) (n int, err error)

	// Reconcile performs consistency checks between metadata/index and blob
	// storage, deleting orphans on either side. It should be idempotent and
	// safe to run periodically.
	Reconcile(ctx context.Context) error
}
