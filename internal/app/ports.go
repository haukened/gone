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

// SecretStatus describes a pending secret as reported to its sender. There is
// deliberately no "opened" or "revoked" state: a secret is pending or gone.
type SecretStatus struct {
	CreatedAt time.Time // when the secret was stored
	ExpiresAt time.Time // TTL deadline
}

// RequestStatus describes a request as reported to its requester: waiting for
// a reply, or with a reply ready to open.
type RequestStatus struct {
	Ready     bool      // a reply has been sent and not yet opened
	CreatedAt time.Time // when the request (or, once ready, its reply) was stored
	ExpiresAt time.Time // deadline for a reply, or for opening it once ready
}

// RequestStore is the storage port for secret requests. Implementations
// must make filling atomic (one reply per request) and must never let a reply
// be claimed without its manage hash.
type RequestStore interface {
	// CreateRequest persists a new open request. fillHash and manageHash are
	// hex SHA-256 digests; ttl is kept so a reply gets its own full lifetime.
	CreateRequest(ctx context.Context, id, fillHash, manageHash string, ttl time.Duration, expiresAt time.Time) error
	// RequestOpen returns the expiry of an open request whose fill hash
	// matches, without changing anything. Returns ErrNotFound otherwise.
	RequestOpen(ctx context.Context, id, fillHash string) (time.Time, error)
	// Fill stores the single reply to an open request. 'r' streams exactly
	// 'size' bytes of ciphertext. Returns the reply's expiry or ErrNotFound.
	Fill(ctx context.Context, id, fillHash string, meta Meta, r io.Reader, size int64) (time.Time, error)
	// RequestStatus reports a request to its requester, without changing
	// anything. Returns ErrNotFound for anything not waiting or ready.
	RequestStatus(ctx context.Context, id, manageHash string) (RequestStatus, error)
	// ClaimReply reserves a ready reply for the requester, as SecretStore.Claim.
	ClaimReply(ctx context.Context, id, manageHash, claimHash string, retry bool, claimedUntil time.Time) (Claimed, error)
	// CancelRequest deletes an open request or an unopened reply.
	CancelRequest(ctx context.Context, id, manageHash string) error
}

// SecretStore is the storage port for secrets. Implementations must provide
// durability and the single-consume invariant. They typically coordinate an
// index (e.g. SQLite) with blob storage (filesystem) but those details are
// outside this interface.
type SecretStore interface {
	// Save persists a new secret blob with metadata and an absolute expiry.
	// 'r' streams exactly 'size' bytes of ciphertext. manageHash is the hex
	// SHA-256 of the sender's manage token. The call MUST return only after
	// the data and metadata are crash-safe (fsync / committed).
	Save(ctx context.Context, id string, meta Meta, manageHash string, r io.Reader, size int64, expiresAt time.Time) error

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

	// Status returns the creation and expiry times of a pending secret whose
	// manage hash matches. It returns ErrNotFound for absent, expired,
	// lapsed-claim, unmanaged, or mismatched secrets, without distinction.
	// A secret under an active claim lease is still pending.
	Status(ctx context.Context, id, manageHash string) (SecretStatus, error)

	// Revoke permanently deletes a pending secret (metadata and payload) whose
	// manage hash matches, even during an active claim lease. It returns
	// ErrNotFound under the same conditions as Status.
	Revoke(ctx context.Context, id, manageHash string) error

	// DeleteExpired removes (or tombstones) secrets whose expiry is <= t and
	// returns the count of secrets affected. Best-effort cleanup of blob files
	// is acceptable; failures should be surfaced via error.
	DeleteExpired(ctx context.Context, t time.Time) (n int, err error)

	// Reconcile performs consistency checks between metadata/index and blob
	// storage, deleting orphans on either side. It should be idempotent and
	// safe to run periodically.
	Reconcile(ctx context.Context) error
}
