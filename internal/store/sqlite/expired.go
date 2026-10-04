package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/haukened/gone/internal/sqlrows"
	"github.com/haukened/gone/internal/store"
)

// DeleteExpired deletes secrets whose TTL has elapsed (expires_at <= t) or
// whose claim lease has elapsed without acknowledgement (claimed_until <= t).
// It returns records for blob cleanup; Claimed is set on rows removed because
// of a lapsed claim.
func (i *Index) DeleteExpired(ctx context.Context, t time.Time) ([]store.ExpiredRecord, error) {
	release, err := i.acquireWrite(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	recs, err := deleteExpiredTxn(ctx, i.db, t)
	return recs, busyError(err)
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
func selectExpired(ctx context.Context, q sqlrows.Querier, t time.Time) (recs []store.ExpiredRecord, err error) {
	const sel = `SELECT id, external, claim_hash IS NOT NULL FROM secrets WHERE ` + expiredWhere
	err = sqlrows.Query(ctx, q, sel, func(r sqlrows.Rows) error {
		var rec store.ExpiredRecord
		if err := r.Scan(&rec.ID, &rec.External, &rec.Claimed); err != nil {
			return err
		}
		recs = append(recs, rec)
		return nil
	}, t.Unix(), t.Unix())
	if err != nil {
		return nil, err
	}
	return recs, nil
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

// ListExternalIDs returns IDs of secrets with external (blob) storage.
func (i *Index) ListExternalIDs(ctx context.Context) (ids []string, err error) {
	err = sqlrows.Query(ctx, i.db, `SELECT id FROM secrets WHERE external=1`, func(r sqlrows.Rows) error {
		var id string
		if err := r.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
