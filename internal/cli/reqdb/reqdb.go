// Package reqdb keeps the secret requests made by the gone CLI on this
// device, in a SQLite database in the user's config directory. Each row holds
// what only the requester may know: the request's manage token and its
// private key, which never appear in a link (docs/protocol.md section 7.5).
// The file is created owner-only (0600) in an owner-only directory, and
// deleted rows are overwritten (secure_delete), so an opened or cancelled
// request leaves no key behind in the file.
package reqdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	// database/sql SQLite driver (pure Go, no CGO)
	_ "modernc.org/sqlite"
)

// FileName is the database file inside the CLI config directory.
const FileName = "requests.db"

// States a request can be in.
const (
	StateWaiting = "waiting"
	StateReady   = "ready"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
	// minPrefix is the shortest ID prefix Resolve accepts.
	minPrefix = 4
)

// ErrNoMatch reports that no saved request matches an ID or prefix.
var ErrNoMatch = errors.New("no saved request matches")

// prefixRE accepts 4 to 32 lowercase hex characters.
var prefixRE = regexp.MustCompile(`^[0-9a-f]{4,32}$`)

// AmbiguousError reports a prefix that matches more than one request.
type AmbiguousError struct {
	// IDs are the matching request IDs.
	IDs []string
}

// Error describes the ambiguity.
func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("the ID prefix matches %d requests", len(e.IDs))
}

// ErrBadPrefix reports an ID argument that is not 4 to 32 hex characters.
var ErrBadPrefix = errors.New("a request ID is 4 to 32 lowercase hex characters")

// Row is one saved request.
type Row struct {
	ID          string    // request ID (32 hex)
	Origin      string    // server origin the request was made on
	Label       string    // the requester's own note; never sent anywhere
	ManageToken string    // authorizes status, the claim, and cancel
	PrivateKey  []byte    // raw P-256 scalar
	ReplyLink   string    // the link to send; it cannot open anything
	State       string    // StateWaiting or StateReady, as last seen
	CreatedAt   time.Time // when the request was made
	ExpiresAt   time.Time // reply deadline, or the reply's own expiry once ready
}

// DB is an open request database.
type DB struct{ db *sql.DB }

// Open opens (creating if needed) the request database in dir. The
// directory is made owner-only and the file is created 0600 before SQLite
// touches it, then its mode is enforced again.
//
// Parameters:
//   - dir: the CLI config directory (…/gone).
//
// Returns the database or an error.
func Open(dir string) (*DB, error) {
	path, err := prepareFile(dir)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=secure_delete(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{db: db}, nil
}

// prepareFile makes dir and the database file owner-only.
//
// Parameters:
//   - dir: the CLI config directory.
//
// Returns the database path or an error.
func prepareFile(dir string) (string, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm) // #nosec G304 -- fixed name under the user config dir
	switch {
	case err == nil:
		if err = f.Close(); err != nil {
			return "", err
		}
	case !errors.Is(err, fs.ErrExist):
		return "", err
	}
	return path, os.Chmod(path, filePerm)
}

// schema is version 1 of the requests table.
const schema = `CREATE TABLE IF NOT EXISTS requests (
id TEXT PRIMARY KEY,
origin TEXT NOT NULL,
label TEXT NOT NULL,
manage_token TEXT NOT NULL,
private_key BLOB NOT NULL,
reply_link TEXT NOT NULL,
state TEXT NOT NULL,
created_at INTEGER NOT NULL,
expires_at INTEGER NOT NULL
);
PRAGMA user_version = 1;`

// migrate brings the schema to version 1, refusing a newer file.
//
// Parameters:
//   - ctx: context.
//   - db: the database.
//
// Returns an error if the schema is newer than this binary or DDL fails.
func migrate(ctx context.Context, db *sql.DB) error {
	var v int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > 1 {
		return errors.New("requests database was written by a newer gone")
	}
	if v == 1 {
		return nil
	}
	_, err := db.ExecContext(ctx, schema)
	return err
}

// Close closes the database.
//
// Returns the close error.
func (d *DB) Close() error { return d.db.Close() }

// Put saves or replaces a request.
//
// Parameters:
//   - ctx: context.
//   - r: the request.
//
// Returns the write error, if any.
func (d *DB) Put(ctx context.Context, r Row) error {
	const q = `INSERT OR REPLACE INTO requests (id, origin, label, manage_token, private_key, reply_link, state, created_at, expires_at) VALUES (?,?,?,?,?,?,?,?,?)`
	_, err := d.db.ExecContext(ctx, q, r.ID, r.Origin, r.Label, r.ManageToken, r.PrivateKey, r.ReplyLink, r.State,
		r.CreatedAt.Unix(), r.ExpiresAt.Unix())
	return err
}

// SetState records the last seen state and expiry of a request.
//
// Parameters:
//   - ctx: context.
//   - id: request ID.
//   - state: StateWaiting or StateReady.
//   - expires: the expiry the server reported.
//
// Returns the write error, if any.
func (d *DB) SetState(ctx context.Context, id, state string, expires time.Time) error {
	_, err := d.db.ExecContext(ctx, `UPDATE requests SET state=?, expires_at=? WHERE id=?`, state, expires.Unix(), id)
	return err
}

// Delete forgets a request and its key.
//
// Parameters:
//   - ctx: context.
//   - id: request ID.
//
// Returns the write error, if any.
func (d *DB) Delete(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM requests WHERE id=?`, id)
	return err
}

// List deletes requests past their expiry, then returns the rest newest
// first.
//
// Parameters:
//   - ctx: context.
//   - now: current time.
//
// Returns the live requests or an error.
func (d *DB) List(ctx context.Context, now time.Time) ([]Row, error) {
	if _, err := d.db.ExecContext(ctx, `DELETE FROM requests WHERE expires_at <= ?`, now.Unix()); err != nil {
		return nil, err
	}
	return d.query(ctx, `ORDER BY created_at DESC, id`)
}

// Resolve finds the one live request whose ID starts with prefix.
//
// Parameters:
//   - ctx: context.
//   - prefix: 4 to 32 lowercase hex characters.
//   - now: current time; expired requests are deleted first.
//
// Returns the request, or ErrBadPrefix, ErrNoMatch, *AmbiguousError, or a
// database error.
func (d *DB) Resolve(ctx context.Context, prefix string, now time.Time) (Row, error) {
	prefix = strings.ToLower(prefix)
	if len(prefix) < minPrefix || !prefixRE.MatchString(prefix) {
		return Row{}, ErrBadPrefix
	}
	rows, err := d.List(ctx, now)
	if err != nil {
		return Row{}, err
	}
	var hits []Row
	for _, r := range rows {
		if strings.HasPrefix(r.ID, prefix) {
			hits = append(hits, r)
		}
	}
	return pick(hits)
}

// pick returns the single row of hits.
//
// Parameters:
//   - hits: matching rows.
//
// Returns the row, ErrNoMatch, or *AmbiguousError.
func pick(hits []Row) (Row, error) {
	switch len(hits) {
	case 0:
		return Row{}, ErrNoMatch
	case 1:
		return hits[0], nil
	}
	e := &AmbiguousError{}
	for _, h := range hits {
		e.IDs = append(e.IDs, h.ID)
	}
	return Row{}, e
}

// query selects rows with the given trailing clause.
//
// Parameters:
//   - ctx: context.
//   - tail: SQL after the FROM clause (a fixed string, never user input).
//
// Returns the rows or an error.
func (d *DB) query(ctx context.Context, tail string) ([]Row, error) {
	rs, err := d.db.QueryContext(ctx, `SELECT id, origin, label, manage_token, private_key, reply_link, state, created_at, expires_at FROM requests `+tail) // #nosec G202 -- tail is a constant
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	var out []Row
	for rs.Next() {
		var (
			r                Row
			created, expires int64
		)
		if err := rs.Scan(&r.ID, &r.Origin, &r.Label, &r.ManageToken, &r.PrivateKey, &r.ReplyLink, &r.State, &created, &expires); err != nil {
			return nil, err
		}
		r.CreatedAt, r.ExpiresAt = time.Unix(created, 0).UTC(), time.Unix(expires, 0).UTC()
		out = append(out, r)
	}
	return out, rs.Err()
}
