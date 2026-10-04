// Package sqlite provides a SQLite-backed implementation of the store.Index
// port for persisting secret metadata and inline ciphertext.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/haukened/gone/internal/store"

	// database/sql SQLite driver (pure Go, no CGO)
	_ "modernc.org/sqlite"
)

// DriverName is the database/sql driver name registered by modernc.org/sqlite.
const DriverName = "sqlite"

var _ store.Index = (*Index)(nil)

// Index implements store.Index using SQLite (via database/sql). It is safe for
// concurrent use. Reads run in parallel; writes queue for writeGate first (see
// acquireWrite), because SQLite allows only one writer at a time.
type Index struct {
	db        *sql.DB
	writeGate chan struct{}
}

// New constructs an Index, initializing the required schema if absent.
func New(db *sql.DB) (*Index, error) {
	ix := &Index{db: db, writeGate: make(chan struct{}, 1)}
	if err := ix.init(); err != nil {
		return nil, err
	}
	return ix, nil
}

// init creates the base secrets table if absent and then applies any pending
// schema migrations (see migrate.go).
//
// Returns an error if DDL fails or the database schema is newer than this
// binary supports (ErrSchemaTooNew).
func (i *Index) init() error {
	ctx := context.Background()
	if _, err := i.db.ExecContext(ctx, baseSchema); err != nil {
		return err
	}
	return i.migrate(ctx)
}

// Insert stores a new secret row.
//
// Parameters:
//   - ctx: request context.
//   - row: the row to store; see store.NewRow for field meanings.
//
// Returns an error if the insert fails (e.g. duplicate id).
func (i *Index) Insert(ctx context.Context, row store.NewRow) error {
	const q = `INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at, manage_hash) VALUES (?,?,?,?,?,?,?,?,?)`
	ext := 0
	if row.External {
		ext = 1
	}
	release, err := i.acquireWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	_, err = i.db.ExecContext(ctx, q, row.ID, row.Meta.Version, row.Meta.NonceB64u, row.Inline, ext, row.Size,
		row.CreatedAt.Unix(), row.ExpiresAt.Unix(), row.ManageHash)
	return busyError(err)
}
