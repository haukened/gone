// Package sqlite provides a SQLite-backed implementation of the store.Index
// port for persisting secret metadata and inline ciphertext.
package sqlite

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
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

// init creates the secrets table if absent and applies idempotent column
// migrations for databases created by earlier versions.
//
// Returns an error if any DDL statement fails.
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
claim_hash TEXT,
claimed_until INTEGER
);`
	if _, err := i.db.Exec(schema); err != nil {
		return err
	}
	return i.migrateClaimColumns()
}

// migrateClaimColumns adds the claim_hash and claimed_until columns to a
// pre-existing secrets table that lacks them. It is safe to call repeatedly.
//
// Returns an error if introspection or ALTER TABLE fails.
func (i *Index) migrateClaimColumns() error {
	cols, err := i.columnNames()
	if err != nil {
		return err
	}
	adds := []struct{ name, ddl string }{
		{"claim_hash", `ALTER TABLE secrets ADD COLUMN claim_hash TEXT`},
		{"claimed_until", `ALTER TABLE secrets ADD COLUMN claimed_until INTEGER`},
	}
	for _, a := range adds {
		if _, ok := cols[a.name]; ok {
			continue
		}
		if _, err := i.db.Exec(a.ddl); err != nil {
			return err
		}
	}
	return nil
}

// columnNames returns the set of column names present on the secrets table.
//
// Returns the set or an error if PRAGMA table_info fails.
func (i *Index) columnNames() (map[string]struct{}, error) {
	rows, err := i.db.Query(`SELECT name FROM pragma_table_info('secrets')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[name] = struct{}{}
	}
	return cols, rows.Err()
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
	var res *store.IndexResult
	err := i.immediateTx(ctx, func(conn *sql.Conn) error {
		var err error
		if res, err = loadLiveRow(ctx, conn, id, now); err != nil {
			return err
		}
		if err = applyClaim(ctx, conn, res, id, claimHash, retry, claimedUntil); err != nil {
			return err
		}
		return attachExternalReader(res, id, openExternal)
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
func (i *Index) immediateTx(ctx context.Context, fn func(conn *sql.Conn) error) error {
	conn, err := i.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
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
		return err
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
	const sel = `SELECT version, nonce_b64u, inline, external, size, expires_at, claim_hash, claimed_until FROM secrets WHERE id=?`
	var (
		res          store.IndexResult
		extInt       int
		expiresUnix  int64
		claimHash    sql.NullString
		claimedUntil sql.NullInt64
	)
	row := q.QueryRowContext(ctx, sel, id)
	if err := row.Scan(&res.Meta.Version, &res.Meta.NonceB64u, &res.Inline, &extInt, &res.Size, &expiresUnix, &claimHash, &claimedUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app.ErrNotFound
		}
		return nil, err
	}
	res.External = extInt == 1
	res.ExpiresAt = time.Unix(expiresUnix, 0).UTC()
	if claimHash.Valid {
		res.ClaimHash = claimHash.String
	}
	if claimedUntil.Valid {
		res.ClaimedUntil = time.Unix(claimedUntil.Int64, 0).UTC()
	}
	return &res, nil
}

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
func scanExpiredRows(rows *sql.Rows) ([]store.ExpiredRecord, error) {
	defer rows.Close()
	var recs []store.ExpiredRecord
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
