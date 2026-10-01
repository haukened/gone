// Package sqlite provides a SQLite-backed implementation of the store.Index
// port for persisting secret metadata and inline ciphertext.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"

	// database/sql SQLite driver (pure Go, no CGO)
	_ "modernc.org/sqlite"
)

// DriverName is the database/sql driver name registered by modernc.org/sqlite.
const DriverName = "sqlite"

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

// Consume hard-deletes the row and returns its data if it existed and is not expired.
// External blobs are opened before the delete is committed; if opening fails,
// the transaction rolls back and the metadata row remains retryable.
func (i *Index) Consume(ctx context.Context, id string, now time.Time, openExternal store.ExternalOpener) (*store.IndexResult, error) {
	conn, err := i.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, err
	}
	var opened io.ReadCloser
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
			if opened != nil {
				_ = opened.Close()
			}
		}
	}()
	res, err := selectSecretForConsume(ctx, conn, id)
	if err != nil {
		return nil, err
	}
	if !res.ExpiresAt.IsZero() && !now.Before(res.ExpiresAt) {
		if err = deleteSecret(ctx, conn, id); err != nil {
			return nil, err
		}
		if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
			return nil, err
		}
		committed = true
		return nil, app.ErrNotFound
	}
	if res.External {
		if openExternal == nil {
			return nil, errors.New("external opener required")
		}
		opened, err = openExternal(id)
		if err != nil {
			return nil, err
		}
		res.Reader = opened
	}
	if err = deleteSecret(ctx, conn, id); err != nil {
		return nil, err
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return nil, err
	}
	committed = true
	return res, nil
}

func selectSecretForConsume(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (*store.IndexResult, error) {
	const sel = `SELECT version, nonce_b64u, inline, external, size, expires_at FROM secrets WHERE id=?`
	var (
		res         store.IndexResult
		extInt      int
		expiresUnix int64
	)
	row := q.QueryRowContext(ctx, sel, id)
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

func deleteSecret(ctx context.Context, e interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, id string) error {
	const del = `DELETE FROM secrets WHERE id=?`
	result, err := e.ExecContext(ctx, del, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

// DeleteExpired selects secrets expiring before t and deletes them, returning records for blob cleanup.
func (i *Index) DeleteExpired(ctx context.Context, t time.Time) ([]store.ExpiredRecord, error) {
	return deleteExpiredTxn(ctx, i.db, t)
}

// deleteExpiredTxn performs the DeleteExpired logic; isolated to reduce cyclomatic complexity on the method receiver.
func deleteExpiredTxn(ctx context.Context, db *sql.DB, t time.Time) ([]store.ExpiredRecord, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	// Ensure rollback on any error prior to successful commit.
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	recs, err := selectExpired(ctx, tx, t)
	if err != nil {
		return nil, err
	}
	if err = deleteExpired(ctx, tx, t); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return recs, nil
}

func selectExpired(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, t time.Time) ([]store.ExpiredRecord, error) {
	const sel = `SELECT id, external FROM secrets WHERE expires_at < ?`
	rows, err := q.QueryContext(ctx, sel, t.Unix())
	if err != nil {
		return nil, err
	}
	return scanExpiredRows(rows)
}

func deleteExpired(ctx context.Context, e interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, t time.Time) error {
	const del = `DELETE FROM secrets WHERE expires_at < ?`
	_, err := e.ExecContext(ctx, del, t.Unix())
	return err
}

// scanExpiredRows reads all rows (id, external) from the provided *sql.Rows into a
// slice of ExpiredRecord. It always closes the rows. The returned slice may be
// empty if no rows were present. An error is returned if scanning or rows.Err()
// produces an error.
func scanExpiredRows(rows *sql.Rows) ([]store.ExpiredRecord, error) {
	defer rows.Close()
	var recs []store.ExpiredRecord
	for rows.Next() {
		var r store.ExpiredRecord
		var extInt int
		if err := rows.Scan(&r.ID, &extInt); err != nil {
			return nil, err
		}
		r.External = extInt == 1
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
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
