package sqlite

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

// Claim reserves a secret for delivery to a single client without deleting it.
//
// When retry is false the caller is requesting a fresh claim: the row must be
// unexpired and unclaimed, after which claim_hash and claimed_until are set.
// When retry is true the caller is re-presenting a previously issued token:
// the stored claim hash must match claimHash and the lease must still be valid.
// Rows whose TTL or claim lease has elapsed are deleted and reported as
// app.ErrNotFound. External blobs are opened inside the transaction so a
// missing blob rolls back the claim.
//
// Parameters:
//   - ctx: request context.
//   - id: secret identifier.
//   - claimHash: hex SHA-256 of the claim token.
//   - retry: whether the caller is re-presenting an existing token.
//   - now: current time used for expiry checks.
//   - claimedUntil: lease deadline stored for a fresh claim.
//   - openExternal: opener for blob payloads (required if the row is external).
//
// Returns the claimed row (with ClaimedUntil set) or an error.
func (i *Index) Claim(ctx context.Context, id, claimHash string, retry bool, now, claimedUntil time.Time, openExternal store.ExternalOpener) (*store.IndexResult, error) {
	return i.claimWhere(ctx, claimRequest{id: id, claimHash: claimHash, retry: retry, now: now, until: claimedUntil},
		func(res *store.IndexResult) bool { return !res.Reply }, openExternal)
}

// ClaimReply reserves a request reply for the requester. It behaves exactly
// like Claim, except that the row must be a reply whose manage hash matches;
// ordinary secrets and mismatched hashes are reported as app.ErrNotFound.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the requester's manage token.
//   - claimHash: hex SHA-256 of the claim token.
//   - retry: whether the caller is re-presenting an existing token.
//   - now: current time used for expiry checks.
//   - claimedUntil: lease deadline stored for a fresh claim.
//   - openExternal: opener for blob payloads (required if the row is external).
//
// Returns the claimed row or an error.
func (i *Index) ClaimReply(ctx context.Context, id, manageHash, claimHash string, retry bool, now, claimedUntil time.Time, openExternal store.ExternalOpener) (*store.IndexResult, error) {
	return i.claimWhere(ctx, claimRequest{id: id, claimHash: claimHash, retry: retry, now: now, until: claimedUntil},
		func(res *store.IndexResult) bool { return res.Reply && hashMatches(res.ManageHash, manageHash) }, openExternal)
}

// claimRequest bundles the inputs of one claim.
type claimRequest struct {
	id        string
	claimHash string
	retry     bool
	now       time.Time
	until     time.Time
}

// claimWhere runs the claim transaction for rows that allow accepts.
//
// Parameters:
//   - ctx: request context.
//   - c: claim inputs.
//   - allow: reports whether a live row may be claimed on this route.
//   - openExternal: opener for blob payloads.
//
// Returns the claimed row or an error.
func (i *Index) claimWhere(ctx context.Context, c claimRequest, allow func(*store.IndexResult) bool, openExternal store.ExternalOpener) (*store.IndexResult, error) {
	var res *store.IndexResult
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		var err error
		if res, err = loadLiveRow(ctx, conn, c.id, c.now); err != nil {
			return err
		}
		if !allow(res) {
			return app.ErrNotFound
		}
		if err = applyClaim(ctx, conn, res, c.id, c.claimHash, c.retry, c.until); err != nil {
			return err
		}
		return attachExternalReader(res, c.id, openExternal)
	})
	if err != nil {
		if res != nil && res.Reader != nil {
			_ = res.Reader.Close()
		}
		return nil, err
	}
	return res, nil
}

// errDeadRow signals from inside a transaction that an expired or lapsed row
// was deleted: the deletion must be committed and app.ErrNotFound returned.
var errDeadRow = errors.New("sqlite: dead row deleted")

// execer is the subset of *sql.Conn / *sql.Tx used to run statements.
type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// immediateTx runs fn inside a BEGIN IMMEDIATE transaction on a dedicated
// connection, taking SQLite's write lock up front so concurrent claims and acks
// serialize. The transaction is committed when fn returns nil or errDeadRow
// (in which case app.ErrNotFound is returned); otherwise, or if fn panics, it
// is rolled back.
//
// Parameters:
//   - ctx: request context.
//   - fn: transactional work bound to the connection.
//
// Returns fn's error, app.ErrNotFound for errDeadRow, or a DB error.
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
	if fnErr != nil && !errors.Is(fnErr, errDeadRow) {
		return fnErr
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return busyError(err)
	}
	committed = true
	if fnErr != nil {
		return app.ErrNotFound
	}
	return nil
}

// loadLiveRow loads a row for claiming, deleting it if its TTL or claim lease
// has elapsed.
//
// Parameters:
//   - ctx: request context.
//   - conn: connection bound to the open transaction.
//   - id: secret identifier.
//   - now: current time used for expiry checks.
//
// Returns the live row, app.ErrNotFound if absent, errDeadRow if it was
// deleted, or a DB error.
func loadLiveRow(ctx context.Context, conn *sql.Conn, id string, now time.Time) (*store.IndexResult, error) {
	res, err := selectSecretForClaim(ctx, conn, id)
	if err != nil {
		return nil, err
	}
	if !isDead(res, now) {
		return res, nil
	}
	if err = deleteSecret(ctx, conn, id); err != nil {
		return nil, err
	}
	return nil, errDeadRow
}

// attachExternalReader opens the blob for an external row and sets res.Reader.
// Inline rows are left unchanged.
//
// Parameters:
//   - res: the claimed row.
//   - id: secret identifier.
//   - openExternal: blob opener (required if the row is external).
//
// Returns an error if the opener is missing or the blob cannot be opened.
func attachExternalReader(res *store.IndexResult, id string, openExternal store.ExternalOpener) error {
	if !res.External {
		return nil
	}
	if openExternal == nil {
		return errors.New("external opener required")
	}
	r, err := openExternal(id)
	if err != nil {
		return err
	}
	res.Reader = r
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

// applyClaim validates claim ownership for a live row and, for a fresh claim,
// persists the claim hash and lease. On success res.ClaimedUntil reflects the
// active lease.
//
// Parameters:
//   - ctx: request context.
//   - e: executor bound to the open transaction.
//   - res: the live row.
//   - id: secret identifier.
//   - claimHash: hex SHA-256 of the presented or newly issued token.
//   - retry: whether the caller is re-presenting an existing token.
//   - claimedUntil: lease deadline for a fresh claim.
//
// Returns app.ErrNotFound if ownership cannot be established, or a DB error.
func applyClaim(ctx context.Context, e execer, res *store.IndexResult, id, claimHash string, retry bool, claimedUntil time.Time) error {
	if retry {
		if !hashMatches(res.ClaimHash, claimHash) {
			return app.ErrNotFound
		}
		return nil
	}
	if res.ClaimHash != "" {
		return app.ErrNotFound
	}
	const upd = `UPDATE secrets SET claim_hash=?, claimed_until=? WHERE id=? AND claim_hash IS NULL`
	if _, err := e.ExecContext(ctx, upd, claimHash, claimedUntil.Unix(), id); err != nil {
		return err
	}
	res.ClaimHash = claimHash
	res.ClaimedUntil = time.Unix(claimedUntil.Unix(), 0).UTC()
	return nil
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
