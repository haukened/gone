package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
)

// dbUserVersion reads PRAGMA user_version through a *sql.DB.
func dbUserVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	return v
}

// assertFullyMigrated checks the version and every migrated column.
func assertFullyMigrated(t *testing.T, db *sql.DB) {
	t.Helper()
	if got := dbUserVersion(t, db); got != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", got, SchemaVersion)
	}
	for _, col := range []string{"claim_hash", "claimed_until", "manage_hash"} {
		if !hasColumn(t, db, col) {
			t.Fatalf("missing column %q", col)
		}
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			now := time.Now().UTC()
			if tc.setup != "" {
				if _, err := db.Exec(tc.setup); err != nil {
					t.Fatalf("setup: %v", err)
				}
				if _, err := db.Exec(`INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at) VALUES ('legacy',1,'n',x'00',0,1,?,?)`, now.Unix(), now.Add(time.Hour).Unix()); err != nil {
					t.Fatalf("insert legacy: %v", err)
				}
			}
			ix, err := New(db)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			assertFullyMigrated(t, db)
			if tc.setup == "" {
				return
			}
			if !rowExists(t, db, "legacy") {
				t.Fatalf("legacy row lost in migration")
			}
			if _, err = ix.Status(context.Background(), "legacy", testManageHash, now); !errors.Is(err, app.ErrNotFound) {
				t.Fatalf("legacy row Status err = %v, want not found", err)
			}
		})
	}
}

func TestMigrateReopenIsNoop(t *testing.T) {
	db := openTestDB(t)
	if _, err := New(db); err != nil {
		t.Fatalf("first New: %v", err)
	}
	if _, err := New(db); err != nil {
		t.Fatalf("second New: %v", err)
	}
	assertFullyMigrated(t, db)
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	db := openTestDB(t)
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
	db := openTestDB(t)
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
