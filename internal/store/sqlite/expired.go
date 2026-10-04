package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/haukened/gone/internal/store"
)

// DeleteExpired deletes secrets whose TTL has elapsed (expires_at <= t) or
// whose claim lease has elapsed without acknowledgement (claimed_until <= t).
// It returns records for blob cleanup; Claimed is set on rows removed because
// of a lapsed claim.
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

// expiredWhere matches rows past their TTL or past an unacknowledged claim lease.
// It takes two bind parameters, both the cutoff unix time.
const expiredWhere = `expires_at <= ? OR (claimed_until IS NOT NULL AND claimed_until <= ?)`

// selectExpired loads expired row identifiers for cleanup.
//
// Parameters:
//   - ctx: request context.
//   - q: query executor.
//   - t: expiration cutoff.
//
// Returns expired records or a DB error.
func selectExpired(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, t time.Time) ([]store.ExpiredRecord, error) {
	const sel = `SELECT id, external, claim_hash IS NOT NULL FROM secrets WHERE ` + expiredWhere
	rows, err := q.QueryContext(ctx, sel, t.Unix(), t.Unix())
	if err != nil {
		return nil, err
	}
	return scanExpiredRows(rows)
}

// deleteExpired removes expired rows from the index.
//
// Parameters:
//   - ctx: request context.
//   - e: statement executor.
//   - t: expiration cutoff.
//
// Returns a DB error, if any.
func deleteExpired(ctx context.Context, e interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, t time.Time) error {
	const del = `DELETE FROM secrets WHERE ` + expiredWhere
	_, err := e.ExecContext(ctx, del, t.Unix(), t.Unix())
	return err
}

// scanExpiredRows reads all rows (id, external, claimed) from the provided *sql.Rows into a
// slice of ExpiredRecord. It always closes the rows. The returned slice may be
// empty if no rows were present. An error is returned if scanning or rows.Err()
// produces an error.
func scanExpiredRows(rows *sql.Rows) (recs []store.ExpiredRecord, err error) {
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var r store.ExpiredRecord
		var extInt, claimedInt int
		if err := rows.Scan(&r.ID, &extInt, &claimedInt); err != nil {
			return nil, err
		}
		r.External = extInt == 1
		r.Claimed = claimedInt == 1
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

// ListExternalIDs returns IDs of secrets with external (blob) storage.
func (i *Index) ListExternalIDs(ctx context.Context) (ids []string, err error) {
	const q = `SELECT id FROM secrets WHERE external=1`
	rows, err := i.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
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
