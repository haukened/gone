package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
)

// sqliteTestManageHash is the manage hash every test helper inserts.
const sqliteTestManageHash = "4d616e6167652d68617368"

func sqliteOpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db") + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	return db
}

// sqliteInsertInlineSecret inserts an inline row using the public index API.
func sqliteInsertInlineSecret(ctx context.Context, t *testing.T, ix *Index, id string, data []byte, now, expires time.Time) {
	t.Helper()
	if err := ix.Insert(ctx, store.NewRow{ID: id, Meta: app.Meta{Version: 1, NonceB64u: "nonce-" + id}, Inline: data, Size: int64(len(data)), ManageHash: sqliteTestManageHash, CreatedAt: now, ExpiresAt: expires}); err != nil {
		t.Fatalf("insert inline %q: %v", id, err)
	}
}

// sqliteInsertExternalSecret inserts an external row using the public index API.
func sqliteInsertExternalSecret(ctx context.Context, t *testing.T, ix *Index, id string, size int64, now, expires time.Time) {
	t.Helper()
	if err := ix.Insert(ctx, store.NewRow{ID: id, Meta: app.Meta{Version: 2, NonceB64u: "nonce-" + id}, External: true, Size: size, ManageHash: sqliteTestManageHash, CreatedAt: now, ExpiresAt: expires}); err != nil {
		t.Fatalf("insert external %q: %v", id, err)
	}
}

// sqliteRowExists reports whether a row with id remains in the secrets table.
func sqliteRowExists(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM secrets WHERE id=?`, id).Scan(&count); err != nil {
		t.Fatalf("count row %q: %v", id, err)
	}
	return count == 1
}

// sqliteHasColumn reports whether the secrets table has a named column.
func sqliteHasColumn(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info('secrets')`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	}()
	for rows.Next() {
		var got string
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if got == name {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
	return false
}

// sqliteCloseTestDB closes db and fails t if the close fails.
func sqliteCloseTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
}
