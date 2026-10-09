package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

var _ store.RequestIndex = (*Index)(nil)

// requestRow is an open request as stored in the requests table.
type requestRow struct {
	fillHash   string
	manageHash string
	ttl        time.Duration
	createdAt  time.Time
	expiresAt  time.Time
}

// live reports whether the request's reply window is still open at now.
//
// Parameters:
//   - now: current time.
//
// Returns true before the expiry.
func (r *requestRow) live(now time.Time) bool { return now.Before(r.expiresAt) }

// InsertRequest stores a new open request.
//
// Parameters:
//   - ctx: request context.
//   - row: the request to store.
//
// Returns an error if the insert fails (e.g. duplicate id).
func (i *Index) InsertRequest(ctx context.Context, row store.NewRequestRow) error {
	const q = `INSERT INTO requests (id, fill_hash, manage_hash, ttl_seconds, created_at, expires_at) VALUES (?,?,?,?,?,?)`
	release, err := i.acquireWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	_, err = i.db.ExecContext(ctx, q, row.ID, row.FillHash, row.ManageHash, int64(row.TTL/time.Second),
		row.CreatedAt.Unix(), row.ExpiresAt.Unix())
	return busyError(err)
}

// RequestOpen returns the expiry of a live open request whose fill hash
// matches. It is a plain read: it takes no write turn and deletes nothing,
// leaving expired rows to the janitor.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - fillHash: hex SHA-256 of the presented fill token.
//   - now: current time used for expiry checks.
//
// Returns the expiry, app.ErrNotFound, or a DB error.
func (i *Index) RequestOpen(ctx context.Context, id, fillHash string, now time.Time) (time.Time, error) {
	row, err := selectRequest(ctx, i.db, id)
	if err != nil {
		return time.Time{}, err
	}
	if !row.live(now) || !hashMatches(row.fillHash, fillHash) {
		return time.Time{}, app.ErrNotFound
	}
	return row.expiresAt, nil
}

// FillRequest atomically turns a live open request into a reply row. The
// request row is deleted and reply is inserted into secrets with the
// request's ID and manage hash, created now and expiring after the request's
// TTL, in one transaction. Exactly one of two concurrent fills succeeds.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - fillHash: hex SHA-256 of the presented fill token.
//   - now: current time.
//   - reply: payload columns (Meta, Inline, External, Size) of the reply.
//
// Returns the reply's expiry, app.ErrNotFound if the request is not open or
// the hash does not match, or a DB error.
func (i *Index) FillRequest(ctx context.Context, id, fillHash string, now time.Time, reply store.NewRow) (time.Time, error) {
	var expires time.Time
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		row, err := selectRequest(ctx, conn, id)
		if err != nil {
			return err
		}
		if !row.live(now) || !hashMatches(row.fillHash, fillHash) {
			return app.ErrNotFound
		}
		if _, err = conn.ExecContext(ctx, `DELETE FROM requests WHERE id=?`, id); err != nil {
			return err
		}
		reply.ID, reply.Reply, reply.ManageHash = id, true, row.manageHash
		reply.CreatedAt, reply.ExpiresAt = now, now.Add(row.ttl)
		expires = time.Unix(reply.ExpiresAt.Unix(), 0).UTC()
		return insertSecret(ctx, conn, reply)
	})
	if err != nil {
		return time.Time{}, err
	}
	return expires, nil
}

// RequestStatus reports a request to the requester holding its manage token:
// waiting while the request is open, ready once its reply is stored. It is a
// plain read, so polling it costs no writes.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the presented manage token.
//   - now: current time used for expiry checks.
//
// Returns the status, app.ErrNotFound, or a DB error.
func (i *Index) RequestStatus(ctx context.Context, id, manageHash string, now time.Time) (app.RequestStatus, error) {
	row, err := selectRequest(ctx, i.db, id)
	if err == nil {
		if !row.live(now) || !hashMatches(row.manageHash, manageHash) {
			return app.RequestStatus{}, app.ErrNotFound
		}
		return app.RequestStatus{CreatedAt: row.createdAt, ExpiresAt: row.expiresAt}, nil
	}
	if !errors.Is(err, app.ErrNotFound) {
		return app.RequestStatus{}, err
	}
	reply, err := selectManageRow(ctx, i.db, id)
	if err != nil {
		return app.RequestStatus{}, err
	}
	if !reply.reply || isDead(&reply.life, now) || !hashMatches(reply.manageHash, manageHash) {
		return app.RequestStatus{}, app.ErrNotFound
	}
	return app.RequestStatus{Ready: true, CreatedAt: reply.createdAt, ExpiresAt: reply.life.ExpiresAt}, nil
}

// CancelRequest deletes an open request, or an unopened reply, on behalf of
// the requester. Like Revoke it wins over an active claim lease.
//
// Parameters:
//   - ctx: request context.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the presented manage token.
//   - now: current time used for expiry checks.
//
// Returns whether a deleted reply's payload was external, app.ErrNotFound, or
// a DB error.
func (i *Index) CancelRequest(ctx context.Context, id, manageHash string, now time.Time) (bool, error) {
	var external bool
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		var err error
		external, err = cancelInTx(ctx, conn, id, manageHash, now)
		return err
	})
	if err != nil {
		return false, err
	}
	return external, nil
}

// cancelInTx deletes the open request or reply row for CancelRequest.
//
// Parameters:
//   - ctx: request context.
//   - conn: connection bound to the open transaction.
//   - id: request identifier.
//   - manageHash: hex SHA-256 of the presented manage token.
//   - now: current time.
//
// Returns whether a reply payload was external, app.ErrNotFound, or a DB
// error.
func cancelInTx(ctx context.Context, conn *sql.Conn, id, manageHash string, now time.Time) (bool, error) {
	row, err := selectRequest(ctx, conn, id)
	if err == nil {
		if !row.live(now) || !hashMatches(row.manageHash, manageHash) {
			return false, app.ErrNotFound
		}
		_, err = conn.ExecContext(ctx, `DELETE FROM requests WHERE id=?`, id)
		return false, err
	}
	if !errors.Is(err, app.ErrNotFound) {
		return false, err
	}
	reply, err := loadManagedRow(ctx, conn, id, manageHash, now)
	if err != nil {
		return false, err
	}
	if !reply.reply {
		return false, app.ErrNotFound
	}
	return reply.external, deleteSecret(ctx, conn, id)
}

// selectRequest reads one open request row.
//
// Parameters:
//   - ctx: request context.
//   - q: database or connection.
//   - id: request identifier.
//
// Returns the row, app.ErrNotFound if absent, or a DB error.
func selectRequest(ctx context.Context, q rowQuerier, id string) (*requestRow, error) {
	const sel = `SELECT fill_hash, manage_hash, ttl_seconds, created_at, expires_at FROM requests WHERE id=?`
	var (
		row              requestRow
		ttl              int64
		created, expires int64
	)
	err := q.QueryRowContext(ctx, sel, id).Scan(&row.fillHash, &row.manageHash, &ttl, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, app.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.ttl = time.Duration(ttl) * time.Second
	row.createdAt = time.Unix(created, 0).UTC()
	row.expiresAt = time.Unix(expires, 0).UTC()
	return &row, nil
}
