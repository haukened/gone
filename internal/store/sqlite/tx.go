package sqlite

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"github.com/haukened/gone/v3/internal/store"
	"time"
)

// immediateTx runs fn inside a BEGIN IMMEDIATE transaction on a dedicated
// connection, taking SQLite's write lock up front so concurrent claims and acks
// serialize. The transaction is committed when fn returns nil or a
// deadRowError (in which case app.ErrNotFound, or store.ErrDeadExternal for a
// row whose payload was external, is returned); otherwise, or if fn panics,
// it is rolled back.
//
// Parameters:
//   - ctx: request context.
//   - fn: transactional work bound to the connection.
//
// Returns fn's error, the deadRowError's notFound error, or a DB error.
func (i *Index) immediateTx(ctx context.Context, fn func(conn *sql.Conn) error) (err error) {
	release, err := i.acquireWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	conn, err := i.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return busyError(err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	fnErr := fn(conn)
	var dead deadRowError
	if fnErr != nil && !errors.As(fnErr, &dead) {
		return fnErr
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return busyError(err)
	}
	committed = true
	if fnErr != nil {
		return dead.notFound()
	}
	return nil
}

// isDead reports whether a row has passed its TTL or its claim lease.
//
// Parameters:
//   - res: the row loaded for claiming.
//   - now: current time.
//
// Returns true if the row must be deleted rather than delivered.
func isDead(res *store.IndexResult, now time.Time) bool {
	if !res.ExpiresAt.IsZero() && !now.Before(res.ExpiresAt) {
		return true
	}
	return res.ClaimHash != "" && !now.Before(res.ClaimedUntil)
}

// hashMatches reports, in constant time, whether a stored claim hash is
// present and equal to the presented one.
//
// Parameters:
//   - stored: claim hash persisted on the row ("" if unclaimed).
//   - presented: claim hash derived from the caller's token.
//
// Returns true only for a non-empty exact match.
func hashMatches(stored, presented string) bool {
	return stored != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(presented)) == 1
}
