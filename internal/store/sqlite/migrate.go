package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/haukened/gone/internal/sqlrows"
)

// ErrSchemaTooNew is returned by New when the database's user_version is
// greater than the newest migration this binary knows about, which indicates
// the database was written by a newer release (i.e. a downgrade).
var ErrSchemaTooNew = errors.New("sqlite: database schema is newer than this binary supports")

// baseSchema is the original (version 0) secrets table. Later columns are
// added by migrations so that fresh and upgraded databases share one path.
const baseSchema = `CREATE TABLE IF NOT EXISTS secrets (
id TEXT PRIMARY KEY,
version INTEGER NOT NULL,
nonce_b64u TEXT NOT NULL,
inline BLOB,
external INTEGER NOT NULL DEFAULT 0,
size INTEGER NOT NULL,
created_at INTEGER NOT NULL,
expires_at INTEGER NOT NULL
);`

// migration is one ordered schema step. Its target version is its 1-based
// position in migrations; setVersion is the literal PRAGMA that records it.
type migration struct {
	apply      func(ctx context.Context, conn *sql.Conn) error
	setVersion string
}

// migrations lists every schema step in order. Append only; never reorder or
// edit a released entry.
var migrations = []migration{
	{apply: migrateClaimColumns, setVersion: `PRAGMA user_version = 1`},
	{apply: migrateManageHash, setVersion: `PRAGMA user_version = 2`},
}

// SchemaVersion is the user_version a fully migrated database reports. It
// must equal len(migrations); a unit test enforces this.
const SchemaVersion = 2

// migrate applies every pending migration in order. Each step runs in its own
// BEGIN IMMEDIATE transaction together with its user_version bump, and
// re-reads the version under the write lock so concurrent processes starting
// against the same database apply each step exactly once.
//
// Parameters:
//   - ctx: context for the DDL statements.
//
// Returns ErrSchemaTooNew if the database is ahead of this binary, or the
// first migration or DB error encountered.
func (i *Index) migrate(ctx context.Context) error {
	for idx, m := range migrations {
		target := idx + 1
		err := i.immediateTx(ctx, func(conn *sql.Conn) error {
			return applyMigration(ctx, conn, m, target)
		})
		if err != nil {
			return fmt.Errorf("sqlite: migration %d: %w", target, err)
		}
	}
	return nil
}

// applyMigration runs a single migration if the database is below target.
//
// Parameters:
//   - ctx: context for the statements.
//   - conn: connection bound to the open transaction.
//   - m: the migration to apply.
//   - target: the version m produces.
//
// Returns ErrSchemaTooNew if the database is ahead of this binary, or a DB
// error.
func applyMigration(ctx context.Context, conn *sql.Conn, m migration, target int) error {
	v, err := userVersion(ctx, conn)
	if err != nil {
		return err
	}
	if v > SchemaVersion {
		return ErrSchemaTooNew
	}
	if v >= target {
		return nil
	}
	if err = m.apply(ctx, conn); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, m.setVersion)
	return err
}

// userVersion reads PRAGMA user_version.
//
// Parameters:
//   - ctx: context for the query.
//   - conn: connection to query.
//
// Returns the version or a DB error.
func userVersion(ctx context.Context, conn *sql.Conn) (int, error) {
	var v int
	err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v)
	return v, err
}

// migrateClaimColumns (migration 1) adds claim_hash and claimed_until. It
// skips columns that already exist because databases created before the
// migration framework may have them at user_version 0.
//
// Parameters:
//   - ctx: context for the statements.
//   - conn: connection bound to the open transaction.
//
// Returns an error if introspection or ALTER TABLE fails.
func migrateClaimColumns(ctx context.Context, conn *sql.Conn) error {
	return addMissingColumns(ctx, conn, []columnDDL{
		{"claim_hash", `ALTER TABLE secrets ADD COLUMN claim_hash TEXT`},
		{"claimed_until", `ALTER TABLE secrets ADD COLUMN claimed_until INTEGER`},
	})
}

// migrateManageHash (migration 2) adds the nullable manage_hash column. Rows
// created before it keep NULL and can never be managed.
//
// Parameters:
//   - ctx: context for the statements.
//   - conn: connection bound to the open transaction.
//
// Returns an error if introspection or ALTER TABLE fails.
func migrateManageHash(ctx context.Context, conn *sql.Conn) error {
	return addMissingColumns(ctx, conn, []columnDDL{
		{"manage_hash", `ALTER TABLE secrets ADD COLUMN manage_hash TEXT`},
	})
}

// columnDDL pairs a column name with the statement that adds it.
type columnDDL struct{ name, ddl string }

// addMissingColumns executes each DDL whose column is not yet present.
//
// Parameters:
//   - ctx: context for the statements.
//   - conn: connection bound to the open transaction.
//   - adds: columns to ensure.
//
// Returns an error if introspection or ALTER TABLE fails.
func addMissingColumns(ctx context.Context, conn *sql.Conn, adds []columnDDL) error {
	cols, err := columnNames(ctx, conn)
	if err != nil {
		return err
	}
	for _, a := range adds {
		if _, ok := cols[a.name]; ok {
			continue
		}
		if _, err = conn.ExecContext(ctx, a.ddl); err != nil {
			return err
		}
	}
	return nil
}

// columnNames returns the set of column names present on the secrets table.
//
// Parameters:
//   - ctx: context for the query.
//   - conn: connection to query.
//
// Returns the set or an error if PRAGMA table_info fails.
func columnNames(ctx context.Context, conn *sql.Conn) (map[string]struct{}, error) {
	cols := make(map[string]struct{})
	err := sqlrows.Query(ctx, conn, `SELECT name FROM pragma_table_info('secrets')`, func(r sqlrows.Rows) error {
		var name string
		err := r.Scan(&name)
		cols[name] = struct{}{}
		return err
	})
	if err != nil {
		return nil, err
	}
	return cols, nil
}
