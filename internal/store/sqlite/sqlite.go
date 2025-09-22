// Package sqlite provides a SQLite-backed implementation of the store.Index
// port for persisting secret metadata and inline ciphertext.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"

	// database/sql SQLite driver
	_ "github.com/mattn/go-sqlite3"
)

var _ store.Index = (*Index)(nil)

// Index implements store.Index using SQLite (via database/sql). It is safe for
// concurrent use; database/sql manages connection pooling and serialization.
type Index struct{ db *sql.DB }

// New constructs an Index, initializing the required schema if absent.
func New(db *sql.DB) (*Index, error) {
	ix := &Index{db: db}
	if err := ix.init(); err != nil {
		return nil, err
	}
	return ix, nil
}

func (i *Index) init() error {
	schema := `CREATE TABLE IF NOT EXISTS secrets (
id TEXT PRIMARY KEY,
version INTEGER NOT NULL,
nonce_b64u TEXT NOT NULL,
inline BLOB,
external INTEGER NOT NULL DEFAULT 0,
size INTEGER NOT NULL,
created_at INTEGER NOT NULL,
expires_at INTEGER NOT NULL
);`
	_, err := i.db.Exec(schema)
	return err
}

// Insert stores a new secret row.
func (i *Index) Insert(ctx context.Context, id string, meta app.Meta, inline []byte, external bool, size int64, createdAt, expiresAt time.Time) error {
	const q = `INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at) VALUES (?,?,?,?,?,?,?,?)`
	ext := 0
	if external {
		ext = 1
	}
	_, err := i.db.ExecContext(ctx, q, id, meta.Version, meta.NonceB64u, inline, ext, size, createdAt.Unix(), expiresAt.Unix())
	return err
}

// Consume hard-deletes the row and returns its data (including expiry) if it existed.
// Expiration is not interpreted here; callers decide if an expired row constitutes not found.
func (i *Index) Consume(ctx context.Context, id string, _ time.Time) (*store.IndexResult, error) {
	const del = `DELETE FROM secrets WHERE id=? RETURNING version, nonce_b64u, inline, external, size, expires_at`
	var (
		res         store.IndexResult
		extInt      int
		expiresUnix int64
	)
	row := i.db.QueryRowContext(ctx, del, id)
	if err := row.Scan(&res.Meta.Version, &res.Meta.NonceB64u, &res.Inline, &extInt, &res.Size, &expiresUnix); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app.ErrNotFound
		}
		return nil, err
	}
	res.External = extInt == 1
	res.ExpiresAt = time.Unix(expiresUnix, 0).UTC()
	return &res, nil
}

// ExpireBefore selects secrets expiring before t and deletes them, returning records for blob cleanup.
func (i *Index) ExpireBefore(ctx context.Context, t time.Time) ([]store.ExpiredRecord, error) {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const sel = `SELECT id, external FROM secrets WHERE expires_at < ?`
	rows, err := tx.QueryContext(ctx, sel, t.Unix())
	if err != nil {
		return nil, err
	}
	var recs []store.ExpiredRecord
	for rows.Next() {
		var r store.ExpiredRecord
		var extInt int
		if err = rows.Scan(&r.ID, &extInt); err != nil {
			if cErr := rows.Close(); cErr != nil {
				return nil, fmt.Errorf("scan error: %v; close error: %w", err, cErr)
			}
			return nil, err
		}
		r.External = extInt == 1
		recs = append(recs, r)
	}
	if cErr := rows.Close(); cErr != nil {
		return nil, cErr
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	const del = `DELETE FROM secrets WHERE expires_at < ?`
	if _, err = tx.ExecContext(ctx, del, t.Unix()); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return recs, nil
}

// ListExternalIDs returns IDs of secrets with external (blob) storage.
func (i *Index) ListExternalIDs(ctx context.Context) ([]string, error) {
	const q = `SELECT id FROM secrets WHERE external=1`
	rows, err := i.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
