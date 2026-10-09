package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

// Ack completes delivery of a claimed secret by deleting its row, provided the
// presented claim hash matches. Ack is permitted even if the lease has lapsed
// (deletion is always safe) so long as the row still exists.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - claimHash: hex SHA-256 of the claim token.
//
// Returns whether the payload was stored externally (so the caller can delete
// the blob), or app.ErrNotFound if no matching claimed row exists.
func (i *Index) Ack(ctx context.Context, id, claimHash string) (bool, error) {
	var external bool
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		var err error
		external, err = ackRow(ctx, conn, id, claimHash)
		return err
	})
	if err != nil {
		return false, err
	}
	return external, nil
}

// ackRow verifies the claim hash on a row and deletes it.
//
// Parameters:
//   - ctx: request context.
//   - conn: connection bound to the open transaction.
//   - id: secret identifier.
//   - claimHash: hex SHA-256 of the claim token.
//
// Returns whether the payload was external, app.ErrNotFound if the row is
// absent or the hash does not match, or a DB error.
func ackRow(ctx context.Context, conn *sql.Conn, id, claimHash string) (bool, error) {
	var (
		stored sql.NullString
		extInt int
	)
	row := conn.QueryRowContext(ctx, `SELECT claim_hash, external FROM secrets WHERE id=?`, id)
	if err := row.Scan(&stored, &extInt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, app.ErrNotFound
		}
		return false, err
	}
	if !hashMatches(stored.String, claimHash) {
		return false, app.ErrNotFound
	}
	if err := deleteSecret(ctx, conn, id); err != nil {
		return false, err
	}
	return extInt == 1, nil
}

// selectSecretForClaim loads a secret row including claim state.
//
// Parameters:
//   - ctx: request context.
//   - q: query executor (connection or transaction).
//   - id: secret identifier.
//
// Returns the row or app.ErrNotFound if absent.
func selectSecretForClaim(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (*store.IndexResult, error) {
	const sel = `SELECT version, nonce_b64u, inline, external, size, expires_at, claim_hash, claimed_until, reply, manage_hash FROM secrets WHERE id=?`
	var (
		res          store.IndexResult
		extInt       int
		expiresUnix  int64
		claimHash    sql.NullString
		claimedUntil sql.NullInt64
		manageHash   sql.NullString
	)
	row := q.QueryRowContext(ctx, sel, id)
	if err := row.Scan(&res.Meta.Version, &res.Meta.NonceB64u, &res.Inline, &extInt, &res.Size, &expiresUnix, &claimHash, &claimedUntil, &res.Reply, &manageHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app.ErrNotFound
		}
		return nil, err
	}
	res.External = extInt == 1
	res.ManageHash = manageHash.String
	res.ExpiresAt = time.Unix(expiresUnix, 0).UTC()
	if claimHash.Valid {
		res.ClaimHash = claimHash.String
	}
	if claimedUntil.Valid {
		res.ClaimedUntil = time.Unix(claimedUntil.Int64, 0).UTC()
	}
	return &res, nil
}

// deleteSecret removes a row by ID and reports app.ErrNotFound when absent.
//
// Parameters:
//   - ctx: request context.
//   - e: executor bound to the open transaction.
//   - id: secret identifier.
//
// Returns nil, app.ErrNotFound, or a DB error.
func deleteSecret(ctx context.Context, e execer, id string) error {
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
