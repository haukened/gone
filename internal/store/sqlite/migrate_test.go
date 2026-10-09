package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

// sqliteDBUserVersion reads PRAGMA user_version through a *sql.DB.
func sqliteDBUserVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	return v
}

// sqliteAssertFullyMigrated checks the version and every migrated column.
func sqliteAssertFullyMigrated(t *testing.T, db *sql.DB) {
	t.Helper()
	if got := sqliteDBUserVersion(t, db); got != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", got, SchemaVersion)
	}
	for _, col := range []string{"claim_hash", "claimed_until", "manage_hash", "reply"} {
		if !sqliteHasColumn(t, db, col) {
			t.Fatalf("missing column %q", col)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='requests'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("requests table missing: n=%d err=%v", n, err)
	}
}

func TestSchemaVersionMatchesMigrations(t *testing.T) {
	if SchemaVersion != len(migrations) {
		t.Fatalf("SchemaVersion = %d, len(migrations) = %d", SchemaVersion, len(migrations))
	}
}

func TestMigrateUpgradePaths(t *testing.T) {
	tests := []struct {
		name  string
		setup string
	}{
		{name: "fresh database"},
		{name: "v0 without claim columns", setup: baseSchema},
		{name: "v0 with claim columns", setup: baseSchema +
			`ALTER TABLE secrets ADD COLUMN claim_hash TEXT;
ALTER TABLE secrets ADD COLUMN claimed_until INTEGER;`},
		{name: "v1", setup: baseSchema +
			`ALTER TABLE secrets ADD COLUMN claim_hash TEXT;
ALTER TABLE secrets ADD COLUMN claimed_until INTEGER;
PRAGMA user_version = 1;`},
		{name: "v2", setup: baseSchema +
			`ALTER TABLE secrets ADD COLUMN claim_hash TEXT;
ALTER TABLE secrets ADD COLUMN claimed_until INTEGER;
ALTER TABLE secrets ADD COLUMN manage_hash TEXT;
PRAGMA user_version = 2;`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sqliteRunMigrateUpgradePath(t, tc.setup)
		})
	}
}

// sqliteRunMigrateUpgradePath verifies one schema upgrade path.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - setup: optional SQL used to create a legacy schema.
//
// Returns: none; failures abort the test.
func sqliteRunMigrateUpgradePath(t *testing.T, setup string) {
	t.Helper()
	db := sqliteOpenTestDB(t)
	now := time.Now().UTC()
	sqliteApplyMigrateSetup(t, db, setup, now)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sqliteAssertFullyMigrated(t, db)
	sqliteAssertMigratedLegacyRow(t, db, ix, setup, now)
}

// sqliteApplyMigrateSetup applies optional legacy schema SQL.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - db: database under test.
//   - setup: optional SQL used to create a legacy schema.
//   - now: timestamp used for inserted legacy rows.
//
// Returns: none; failures abort the test.
func sqliteApplyMigrateSetup(t *testing.T, db *sql.DB, setup string, now time.Time) {
	t.Helper()
	if setup == "" {
		return
	}
	if _, err := db.Exec(setup); err != nil {
		t.Fatalf("setup: %v", err)
	}
	query := `INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at) VALUES ('legacy',1,'n',x'00',0,1,?,?)`
	if _, err := db.Exec(query, now.Unix(), now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("insert legacy: %v", err)
	}
}

// sqliteAssertMigratedLegacyRow verifies legacy rows survive migration.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - db: database under test.
//   - ix: migrated index.
//   - setup: setup SQL used by the case.
//   - now: current time used for status checks.
//
// Returns: none; failures abort the test.
func sqliteAssertMigratedLegacyRow(t *testing.T, db *sql.DB, ix *Index, setup string, now time.Time) {
	t.Helper()
	if setup == "" {
		return
	}
	if !sqliteRowExists(t, db, "legacy") {
		t.Fatalf("legacy row lost in migration")
	}
	if _, err := ix.Status(context.Background(), "legacy", sqliteTestManageHash, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("legacy row Status err = %v, want not found", err)
	}
}

func TestMigrateReopenIsNoop(t *testing.T) {
	db := sqliteOpenTestDB(t)
	if _, err := New(db); err != nil {
		t.Fatalf("first New: %v", err)
	}
	if _, err := New(db); err != nil {
		t.Fatalf("second New: %v", err)
	}
	sqliteAssertFullyMigrated(t, db)
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	db := sqliteOpenTestDB(t)
	if _, err := New(db); err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	if _, err := New(db); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("New err = %v, want ErrSchemaTooNew", err)
	}
}

func TestMigrateApplyErrorWrapped(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix := &Index{db: db}
	// No secrets table: migration 1 introspects nothing then ALTER TABLE fails.
	err := ix.migrate(context.Background())
	if err == nil {
		t.Fatalf("expected migration error without secrets table")
	}
	if !strings.HasPrefix(err.Error(), "sqlite: migration 1: ") {
		t.Fatalf("error not wrapped with migration number: %q", err)
	}
}
