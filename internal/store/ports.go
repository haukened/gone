// Package store defines internal persistence adapter ports used by the
// higher-level SecretStore implementation. These ports isolate the concrete
// SQLite index and filesystem blob storage so they can be tested and evolved
// independently. Callers outside this package interact only with the
// app.SecretStore implementation, not these internal details.
package store

import (
	"context"
	"io"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

// Index abstracts the metadata/index operations (typically backed by SQLite).
// It stores secret metadata, inlined small ciphertext, and references to blob
// files for larger payloads.
type Index interface {
	// Insert stores a new row described by row.
	Insert(ctx context.Context, row NewRow) error
	// Claim reserves a live secret for a single client and returns its data
	// without deleting it. A fresh claim (retry=false) requires the row to be
	// unclaimed and records claimHash with a lease ending at claimedUntil. A retry
	// (retry=true) requires claimHash to match the stored hash while the lease is
	// valid. Rows past their TTL or claim lease are deleted and app.ErrNotFound is
	// returned. For external payloads, openExternal is called while the
	// transaction is held; if opening fails, the claim is rolled back.
	Claim(ctx context.Context, id, claimHash string, retry bool, now, claimedUntil time.Time, openExternal ExternalOpener) (*IndexResult, error)
	// Ack deletes a claimed row when claimHash matches the stored claim and
	// reports whether its payload was external. Returns app.ErrNotFound otherwise.
	Ack(ctx context.Context, id, claimHash string) (external bool, err error)
	// Status returns a live row's creation and expiry times when manageHash
	// matches. Dead rows are deleted lazily. Returns app.ErrNotFound otherwise.
	Status(ctx context.Context, id, manageHash string, now time.Time) (app.SecretStatus, error)
	// Revoke deletes a live row when manageHash matches, regardless of any
	// claim lease, and reports whether its payload was external. Dead rows are
	// deleted lazily. Returns app.ErrNotFound otherwise.
	Revoke(ctx context.Context, id, manageHash string, now time.Time) (external bool, err error)
	DeleteExpired(ctx context.Context, t time.Time) (expired []ExpiredRecord, err error)
	// ListExternalIDs returns IDs of secrets whose payloads are stored externally.
	ListExternalIDs(ctx context.Context) ([]string, error)
}

// RequestIndex is the optional index extension for secret requests. An open
// request lives in its own table and never holds ciphertext; filling it moves
// it into the secrets table as a reply row, which only the request's manage
// hash can claim.
type RequestIndex interface {
	// InsertRequest stores a new open request.
	InsertRequest(ctx context.Context, row NewRequestRow) error
	// RequestOpen returns the expiry of a live open request whose fill hash
	// matches. It is read-only. Returns app.ErrNotFound otherwise.
	RequestOpen(ctx context.Context, id, fillHash string, now time.Time) (time.Time, error)
	// FillRequest atomically replaces a live open request whose fill hash
	// matches with a reply row built from reply (ID, reply flag, manage hash
	// and timestamps are set here). Returns the reply's expiry, or
	// app.ErrNotFound if the request is not open.
	FillRequest(ctx context.Context, id, fillHash string, now time.Time, reply NewRow) (time.Time, error)
	// RequestStatus reports whether a request is waiting or its reply is
	// ready, when manageHash matches. It is read-only. Returns
	// app.ErrNotFound otherwise.
	RequestStatus(ctx context.Context, id, manageHash string, now time.Time) (app.RequestStatus, error)
	// ClaimReply behaves like Index.Claim but only for a reply row whose
	// manage hash matches.
	ClaimReply(ctx context.Context, id, manageHash, claimHash string, retry bool, now, claimedUntil time.Time, openExternal ExternalOpener) (*IndexResult, error)
	// CancelRequest deletes an open request or an unopened reply whose manage
	// hash matches, and reports whether a reply payload was external.
	CancelRequest(ctx context.Context, id, manageHash string, now time.Time) (external bool, err error)
}

// NewRequestRow describes an open request for RequestIndex.InsertRequest.
type NewRequestRow struct {
	ID         string
	FillHash   string        // hex SHA-256 of the fill token
	ManageHash string        // hex SHA-256 of the requester's manage token
	TTL        time.Duration // reply lifetime, counted again from the fill
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// ExternalOpener opens an external payload without consuming or deleting it.
type ExternalOpener func(id string) (io.ReadCloser, error)

// NewRow describes a secret row for Index.Insert.
type NewRow struct {
	ID         string
	Meta       app.Meta
	Inline     []byte // ciphertext stored in the row; nil for external payloads
	External   bool   // whether the ciphertext lives in blob storage
	Size       int64  // ciphertext size in bytes
	ManageHash string // hex SHA-256 of the sender's manage token
	Reply      bool   // whether the row is a request reply (claimable only via ClaimReply)
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// IndexResult bundles the data returned by Index.Claim.
type IndexResult struct {
	Meta         app.Meta
	Inline       []byte
	Reader       io.ReadCloser
	External     bool
	Size         int64
	ExpiresAt    time.Time
	ClaimHash    string    // hex SHA-256 of the active claim token; empty if unclaimed
	ClaimedUntil time.Time // lease deadline; zero if unclaimed
	Reply        bool      // whether the row is a request reply
	ManageHash   string    // hex SHA-256 of the manage token; empty if unmanaged
}

// BlobStorage abstracts large payload persistence (e.g. filesystem). Reads never
// delete: a claimed blob must survive until the client acknowledges receipt
// (Index.Ack) so interrupted downloads can be retried. Deletion happens via
// Delete on acknowledgement, expiry, or reconciliation; reconciliation uses List
// to clean orphans left by crashes between index removal and blob deletion.
type BlobStorage interface {
	Write(id string, r io.Reader, size int64) error
	// Open returns a reader for the blob without deleting it on close.
	Open(id string) (io.ReadCloser, error)
	// Delete force-removes a blob by id (used by expiry and reconciliation).
	Delete(id string) error
	// List returns all blob IDs present in storage (filenames sans extension).
	List() ([]string, error)
}

// ExpiredRecord represents an expired secret needing blob cleanup (if blobPath non-empty).
type ExpiredRecord struct {
	ID       string
	External bool // true if payload stored in blob storage
	Claimed  bool // true if the row was claimed but never acknowledged
	Request  bool // true if the row was an open request that was never answered
}
