// Package sqlite implements a SQLite-based store, used for indexing secret metadata.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	// Import SQLite3 driver for database/sql
	_ "github.com/mattn/go-sqlite3"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
)

// Ensure Index implements store.Index
var _ store.Index = (*Index)(nil)

// Index implements the store.Index port using SQLite. It manages metadata
// rows and inline payloads. Large payloads live in external blob storage.
type Index struct {
	db *sql.DB
}

// New returns a new SQLite index. The caller is responsible for providing a
// configured *sql.DB (WAL, busy timeout, foreign keys). Schema creation is
// performed if necessary.
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
expires_at INTEGER NOT NULL,
consumed_at INTEGER
);`
	_, err := i.db.Exec(schema)
	return err
}

// Insert implements store.Index.Insert.
func (i *Index) Insert(ctx context.Context, id string, meta app.Meta, inline []byte, external bool, size int64, createdAt, expiresAt time.Time) error {
	const q = `INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at) VALUES (?,?,?,?,?,?,?,?)`
	ext := 0
	if external {
		ext = 1
	}
	_, err := i.db.ExecContext(ctx, q,
		id,
		meta.Version,
		meta.NonceB64u,
		inline,
		ext,
		size,
		createdAt.Unix(),
		expiresAt.Unix(),
	)
	return err
}

// ConsumeOnce atomically marks the secret consumed and returns its data.
func (i *Index) ConsumeOnce(ctx context.Context, id string, now time.Time) (meta app.Meta, inline []byte, external bool, size int64, err error) {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return meta, nil, false, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const sel = `SELECT version, nonce_b64u, inline, external, size, expires_at, consumed_at FROM secrets WHERE id=?`
	var expiresUnix int64
	var consumedAt sql.NullInt64
	var extInt int
	row := tx.QueryRowContext(ctx, sel, id)
	if err = row.Scan(&meta.Version, &meta.NonceB64u, &inline, &extInt, &size, &expiresUnix, &consumedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return meta, nil, false, 0, app.ErrNotFound
		}
		return meta, nil, false, 0, err
	}
	if now.Unix() >= expiresUnix || consumedAt.Valid {
		return meta, nil, false, 0, app.ErrNotFound
	}
	const upd = `UPDATE secrets SET consumed_at=? WHERE id=? AND consumed_at IS NULL`
	if _, err = tx.ExecContext(ctx, upd, now.Unix(), id); err != nil {
		return meta, nil, false, 0, err
	}
	if err = tx.Commit(); err != nil {
		return meta, nil, false, 0, err
	}
	external = extInt == 1
	return meta, inline, external, size, nil
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
			rows.Close()
			return nil, err
		}
		r.External = extInt == 1
		recs = append(recs, r)
	}
	rows.Close()
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
