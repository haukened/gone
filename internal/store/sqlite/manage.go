package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
)

// manageRow is the subset of a secrets row needed by Status and Revoke.
type manageRow struct {
	external   bool
	createdAt  time.Time
	manageHash string
	life       store.IndexResult // ExpiresAt, ClaimHash, ClaimedUntil for isDead
}

// Status reports a pending secret to the sender holding its manage token.
// Rows past their TTL or an unacknowledged claim lease are deleted lazily.
// A secret with an active claim lease is still reported as pending.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - manageHash: hex SHA-256 of the presented manage token.
//   - now: current time used for expiry checks.
//
// Returns the creation and expiry times, or app.ErrNotFound if the row is
// absent, dead, has no manage hash, or the hash does not match.
func (i *Index) Status(ctx context.Context, id, manageHash string, now time.Time) (app.SecretStatus, error) {
	var st app.SecretStatus
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		row, err := loadManagedRow(ctx, conn, id, manageHash, now)
		if err != nil {
			return err
		}
		st = app.SecretStatus{CreatedAt: row.createdAt, ExpiresAt: row.life.ExpiresAt}
		return nil
	})
	if err != nil {
		return app.SecretStatus{}, err
	}
	return st, nil
}

// Revoke deletes a pending secret on behalf of the sender holding its manage
// token. Revoke wins over an active claim lease: the recipient's subsequent
// retry or ack finds no row.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - manageHash: hex SHA-256 of the presented manage token.
//   - now: current time used for expiry checks.
//
// Returns whether the payload was stored externally (so the caller can delete
// the blob), or app.ErrNotFound under the same conditions as Status.
func (i *Index) Revoke(ctx context.Context, id, manageHash string, now time.Time) (bool, error) {
	var external bool
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		row, err := loadManagedRow(ctx, conn, id, manageHash, now)
		if err != nil {
			return err
		}
		if err = deleteSecret(ctx, conn, id); err != nil {
			return err
		}
		external = row.external
		return nil
	})
	if err != nil {
		return false, err
	}
	return external, nil
}

// loadManagedRow loads a row for a manage operation. A dead row is deleted
// (returning errDeadRow so the deletion commits) before the hash is checked,
// matching the lazy-expiry rule of the claim path.
//
// Parameters:
//   - ctx: request context.
//   - conn: connection bound to the open transaction.
//   - id: secret identifier.
//   - manageHash: hex SHA-256 of the presented manage token.
//   - now: current time used for expiry checks.
//
// Returns the live, authorized row, app.ErrNotFound, errDeadRow, or a DB
// error.
func loadManagedRow(ctx context.Context, conn *sql.Conn, id, manageHash string, now time.Time) (*manageRow, error) {
	row, err := selectManageRow(ctx, conn, id)
	if err != nil {
		return nil, err
	}
	if isDead(&row.life, now) {
		if err = deleteSecret(ctx, conn, id); err != nil {
			return nil, err
		}
		return nil, errDeadRow
	}
	if !hashMatches(row.manageHash, manageHash) {
		return nil, app.ErrNotFound
	}
	return row, nil
}

// selectManageRow reads the columns needed for Status and Revoke.
//
// Parameters:
//   - ctx: request context.
//   - conn: connection bound to the open transaction.
//   - id: secret identifier.
//
// Returns the row, app.ErrNotFound if absent, or a DB error.
func selectManageRow(ctx context.Context, conn *sql.Conn, id string) (*manageRow, error) {
	const sel = `SELECT external, created_at, expires_at, claim_hash, claimed_until, manage_hash FROM secrets WHERE id=?`
	var (
		row          manageRow
		extInt       int
		created      int64
		expires      int64
		claimHash    sql.NullString
		claimedUntil sql.NullInt64
		manageHash   sql.NullString
	)
	err := conn.QueryRowContext(ctx, sel, id).Scan(&extInt, &created, &expires, &claimHash, &claimedUntil, &manageHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, app.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.external = extInt == 1
	row.createdAt = time.Unix(created, 0).UTC()
	row.manageHash = manageHash.String
	row.life.ExpiresAt = time.Unix(expires, 0).UTC()
	row.life.ClaimHash = claimHash.String
	if claimedUntil.Valid {
		row.life.ClaimedUntil = time.Unix(claimedUntil.Int64, 0).UTC()
	}
	return &row, nil
}
