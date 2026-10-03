package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
)

// openTestDB opens a transient SQLite database file in a temp dir with WAL enabled.
func openTestDB(t *testing.T) *sql.DB {
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

// insertInlineSecret inserts an inline row using the public index API.
func insertInlineSecret(t *testing.T, ctx context.Context, ix *Index, id string, data []byte, now, expires time.Time) {
	t.Helper()
	if err := ix.Insert(ctx, store.NewRow{ID: id, Meta: app.Meta{Version: 1, NonceB64u: "nonce-" + id}, Inline: data, Size: int64(len(data)), ManageHash: testManageHash, CreatedAt: now, ExpiresAt: expires}); err != nil {
		t.Fatalf("insert inline %q: %v", id, err)
	}
}

// insertExternalSecret inserts an external row using the public index API.
func insertExternalSecret(t *testing.T, ctx context.Context, ix *Index, id string, size int64, now, expires time.Time) {
	t.Helper()
	if err := ix.Insert(ctx, store.NewRow{ID: id, Meta: app.Meta{Version: 2, NonceB64u: "nonce-" + id}, External: true, Size: size, ManageHash: testManageHash, CreatedAt: now, ExpiresAt: expires}); err != nil {
		t.Fatalf("insert external %q: %v", id, err)
	}
}

// rowExists reports whether a row with id remains in the secrets table.
func rowExists(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM secrets WHERE id=?`, id).Scan(&count); err != nil {
		t.Fatalf("count row %q: %v", id, err)
	}
	return count == 1
}

// hasColumn reports whether the secrets table has a named column.
func hasColumn(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info('secrets')`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
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

func TestIndexMigrationAddsClaimColumnsAndPreservesRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	oldSchema := `CREATE TABLE secrets (
id TEXT PRIMARY KEY,
version INTEGER NOT NULL,
nonce_b64u TEXT NOT NULL,
inline BLOB,
external INTEGER NOT NULL DEFAULT 0,
size INTEGER NOT NULL,
created_at INTEGER NOT NULL,
expires_at INTEGER NOT NULL
);`
	if _, err := db.ExecContext(ctx, oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at) VALUES (?,?,?,?,?,?,?,?)`, "legacy", 1, "nonce-legacy", []byte("legacy-data"), 0, len("legacy-data"), now.Unix(), now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !hasColumn(t, db, "claim_hash") || !hasColumn(t, db, "claimed_until") {
		t.Fatalf("expected migration to add claim columns")
	}
	res, err := ix.Claim(ctx, "legacy", "hash-legacy", false, now, now.Add(time.Minute), nil)
	if err != nil {
		t.Fatalf("claim legacy row: %v", err)
	}
	if string(res.Inline) != "legacy-data" {
		t.Fatalf("legacy data got=%q", res.Inline)
	}
}

func TestIndexClaimInlineBehaviors(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		setup    func(t *testing.T, ctx context.Context, ix *Index, db *sql.DB, now time.Time)
		retry    bool
		hash     string
		atOffset time.Duration
		wantErr  error
		check    func(t *testing.T, ctx context.Context, ix *Index, db *sql.DB, now time.Time)
	}{
		{
			name: "fresh claim succeeds",
			id:   "fresh",
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "fresh", []byte("fresh-data"), now, now.Add(time.Hour))
			},
			hash: "hash-fresh",
			check: func(t *testing.T, _ context.Context, _ *Index, _ *sql.DB, _ time.Time) {
				// Success is fully validated by the shared assertions below.
			},
		},
		{
			name: "second fresh claim returns not found",
			id:   "second-fresh",
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "second-fresh", []byte("claimed-data"), now, now.Add(time.Hour))
				if _, err := ix.Claim(ctx, "second-fresh", "hash-owner", false, now, now.Add(time.Minute), nil); err != nil {
					t.Fatalf("initial claim: %v", err)
				}
			},
			hash:    "hash-other",
			wantErr: app.ErrNotFound,
		},
		{
			name: "retry with correct hash succeeds and returns same data",
			id:   "retry-correct",
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "retry-correct", []byte("retry-data"), now, now.Add(time.Hour))
				first, err := ix.Claim(ctx, "retry-correct", "hash-retry", false, now, now.Add(time.Minute), nil)
				if err != nil {
					t.Fatalf("initial claim: %v", err)
				}
				if string(first.Inline) != "retry-data" {
					t.Fatalf("initial data got=%q", first.Inline)
				}
			},
			retry: true,
			hash:  "hash-retry",
		},
		{
			name: "retry with wrong hash returns not found",
			id:   "retry-wrong",
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "retry-wrong", []byte("retry-data"), now, now.Add(time.Hour))
				if _, err := ix.Claim(ctx, "retry-wrong", "hash-owner", false, now, now.Add(time.Minute), nil); err != nil {
					t.Fatalf("initial claim: %v", err)
				}
			},
			retry:   true,
			hash:    "hash-intruder",
			wantErr: app.ErrNotFound,
			check: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				if _, err := ix.Claim(ctx, "retry-wrong", "hash-owner", true, now.Add(2*time.Second), now.Add(time.Minute), nil); err != nil {
					t.Fatalf("correct retry after wrong retry failed: %v", err)
				}
			},
		},
		{
			name: "retry on unclaimed row returns not found",
			id:   "retry-unclaimed",
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "retry-unclaimed", []byte("unclaimed-data"), now, now.Add(time.Hour))
			},
			retry:   true,
			hash:    "hash-missing",
			wantErr: app.ErrNotFound,
		},
		{
			name: "claim after ttl deletes row and returns not found",
			id:   "ttl-dead",
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "ttl-dead", []byte("dead-data"), now.Add(-time.Hour), now)
			},
			hash:    "hash-dead",
			wantErr: app.ErrNotFound,
			check: func(t *testing.T, _ context.Context, _ *Index, db *sql.DB, _ time.Time) {
				if rowExists(t, db, "ttl-dead") {
					t.Fatalf("expected expired row deleted")
				}
			},
		},
		{
			name:     "claim after lease lapsed deletes row and returns not found",
			id:       "lease-dead",
			atOffset: time.Minute,
			setup: func(t *testing.T, ctx context.Context, ix *Index, _ *sql.DB, now time.Time) {
				insertInlineSecret(t, ctx, ix, "lease-dead", []byte("lease-data"), now, now.Add(time.Hour))
				if _, err := ix.Claim(ctx, "lease-dead", "hash-owner", false, now, now.Add(time.Minute), nil); err != nil {
					t.Fatalf("initial claim: %v", err)
				}
			},
			retry:   true,
			hash:    "hash-owner",
			wantErr: app.ErrNotFound,
			check: func(t *testing.T, _ context.Context, _ *Index, db *sql.DB, _ time.Time) {
				if rowExists(t, db, "lease-dead") {
					t.Fatalf("expected lapsed claim row deleted")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			ix, err := New(db)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Second)
			tc.setup(t, ctx, ix, db, now)
			res, err := ix.Claim(ctx, tc.id, tc.hash, tc.retry, now.Add(tc.atOffset), now.Add(5*time.Minute), nil)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				if res != nil {
					t.Fatalf("expected nil result on error")
				}
			} else {
				if err != nil {
					t.Fatalf("Claim: %v", err)
				}
				if res.External {
					t.Fatalf("expected inline result")
				}
				if len(res.Inline) == 0 {
					t.Fatalf("expected inline data")
				}
				if res.ClaimHash != tc.hash {
					t.Fatalf("claim hash got=%q want=%q", res.ClaimHash, tc.hash)
				}
			}
			if tc.check != nil {
				tc.check(t, ctx, ix, db, now)
			}
		})
	}
}

func TestIndexClaimExternalOpensBlob(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "external-open"
	now := time.Now().UTC()
	insertExternalSecret(t, ctx, ix, id, 1234, now, now.Add(time.Hour))
	var openedID string
	var opens int
	res, err := ix.Claim(ctx, id, "hash-ext", false, now, now.Add(time.Minute), func(id string) (io.ReadCloser, error) {
		openedID = id
		opens++
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("Claim external: %v", err)
	}
	defer res.Reader.Close()
	if !res.External {
		t.Fatalf("expected external=true")
	}
	if openedID != id || opens != 1 {
		t.Fatalf("openExternal id=%q opens=%d", openedID, opens)
	}
	if res.Reader == nil {
		t.Fatalf("expected external reader")
	}
	if res.Size != 1234 {
		t.Fatalf("size mismatch")
	}
	got, err := io.ReadAll(res.Reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "blob" {
		t.Fatalf("reader data got=%q", got)
	}
}

func TestIndexClaimExternalOpenFailureLeavesRowUnclaimed(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext-open-failure"
	now := time.Now().UTC()
	insertExternalSecret(t, ctx, ix, id, 1234, now, now.Add(time.Hour))
	openErr := errors.New("open failed")
	if _, err := ix.Claim(ctx, id, "hash-failed", false, now, now.Add(time.Minute), func(string) (io.ReadCloser, error) {
		return nil, openErr
	}); !errors.Is(err, openErr) {
		t.Fatalf("expected open error, got %v", err)
	}
	res, err := ix.Claim(ctx, id, "hash-after-failure", false, now.Add(time.Second), now.Add(time.Minute), func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("expected fresh claim after failed open to succeed, got %v", err)
	}
	if res.Reader != nil {
		_ = res.Reader.Close()
	}
}

func TestIndexClaimExternalConcurrentOnlyOneFreshClaimSucceeds(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext-concurrent"
	now := time.Now().UTC()
	insertExternalSecret(t, ctx, ix, id, 4, now, now.Add(time.Minute))
	var opens int32
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			res, err := ix.Claim(ctx, id, "hash-concurrent-"+string(rune('a'+n)), false, now.Add(time.Second), now.Add(time.Minute), func(string) (io.ReadCloser, error) {
				atomic.AddInt32(&opens, 1)
				time.Sleep(50 * time.Millisecond)
				return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
			})
			if err == nil && res != nil && res.Reader != nil {
				_ = res.Reader.Close()
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	notFound := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, app.ErrNotFound):
			notFound++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if successes != 1 || notFound != 1 {
		t.Fatalf("expected one success and one not found, got success=%d not_found=%d", successes, notFound)
	}
	if got := atomic.LoadInt32(&opens); got != 1 {
		t.Fatalf("expected exactly one external open, got %d", got)
	}
}

func TestIndexAckMatchingHashDeletesRowAndReportsExternal(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		external bool
	}{
		{name: "inline", id: "ack-inline", external: false},
		{name: "external", id: "ack-external", external: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			ix, err := New(db)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx := context.Background()
			now := time.Now().UTC()
			if tc.external {
				insertExternalSecret(t, ctx, ix, tc.id, 77, now, now.Add(time.Hour))
			} else {
				insertInlineSecret(t, ctx, ix, tc.id, []byte("ack-data"), now, now.Add(time.Hour))
			}
			res, err := ix.Claim(ctx, tc.id, "hash-ack", false, now, now.Add(time.Minute), func(string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
			})
			if err != nil {
				t.Fatalf("Claim: %v", err)
			}
			if res.Reader != nil {
				_ = res.Reader.Close()
			}
			external, err := ix.Ack(ctx, tc.id, "hash-ack")
			if err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if external != tc.external {
				t.Fatalf("external got=%v want=%v", external, tc.external)
			}
			if rowExists(t, db, tc.id) {
				t.Fatalf("expected ack to delete row")
			}
		})
	}
}

func TestIndexAckMismatchedHashReturnsNotFoundAndLeavesRow(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	id := "ack-mismatch"
	insertInlineSecret(t, ctx, ix, id, []byte("ack-data"), now, now.Add(time.Hour))
	if _, err := ix.Claim(ctx, id, "hash-owner", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := ix.Ack(ctx, id, "hash-wrong"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if !rowExists(t, db, id) {
		t.Fatalf("expected row to remain after mismatched ack")
	}
	if _, err := ix.Claim(ctx, id, "hash-owner", true, now.Add(time.Second), now.Add(time.Minute), nil); err != nil {
		t.Fatalf("expected correct retry after mismatched ack to succeed: %v", err)
	}
}

func TestIndexAckMissingReturnsNotFound(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := ix.Ack(context.Background(), "missing", "hash"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIndexAckAllowsLapsedLease(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	id := "ack-lapsed"
	insertInlineSecret(t, ctx, ix, id, []byte("ack-data"), now, now.Add(time.Hour))
	if _, err := ix.Claim(ctx, id, "hash-lapsed", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE secrets SET claimed_until=? WHERE id=?`, now.Add(-time.Minute).Unix(), id); err != nil {
		t.Fatalf("force lapsed lease: %v", err)
	}
	if external, err := ix.Ack(ctx, id, "hash-lapsed"); err != nil || external {
		t.Fatalf("Ack after lapsed lease got external=%v err=%v", external, err)
	}
	if rowExists(t, db, id) {
		t.Fatalf("expected ack to delete row")
	}
}

func TestIndexDeleteExpiredRemovesLapsedClaimsAndBoundaryRows(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(time.Minute)
	insertInlineSecret(t, ctx, ix, "ttl-boundary", []byte("ttl"), now, cutoff)
	insertExternalSecret(t, ctx, ix, "claim-boundary", 42, now, now.Add(time.Hour))
	insertInlineSecret(t, ctx, ix, "future", []byte("future"), now, cutoff.Add(time.Second))
	res, err := ix.Claim(ctx, "claim-boundary", "hash-claim-boundary", false, now, cutoff, func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("Claim boundary row: %v", err)
	}
	if res.Reader != nil {
		_ = res.Reader.Close()
	}
	recs, err := ix.DeleteExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 expired records, got %d (%+v)", len(recs), recs)
	}
	byID := map[string]store.ExpiredRecord{}
	for _, rec := range recs {
		byID[rec.ID] = rec
	}
	if rec, ok := byID["ttl-boundary"]; !ok || rec.Claimed || rec.External {
		t.Fatalf("ttl boundary record got=%+v ok=%v", rec, ok)
	}
	if rec, ok := byID["claim-boundary"]; !ok || !rec.Claimed || !rec.External {
		t.Fatalf("claim boundary record got=%+v ok=%v", rec, ok)
	}
	if rowExists(t, db, "ttl-boundary") || rowExists(t, db, "claim-boundary") {
		t.Fatalf("expected boundary rows removed")
	}
	if !rowExists(t, db, "future") {
		t.Fatalf("expected future row to remain")
	}
}

func TestIndexListExternalIDs(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	insertInlineSecret(t, ctx, ix, "inl", []byte("d"), now, now.Add(5*time.Minute))
	insertExternalSecret(t, ctx, ix, "extA", 11, now, now.Add(5*time.Minute))
	insertExternalSecret(t, ctx, ix, "extB", 12, now, now.Add(5*time.Minute))
	ids, err := ix.ListExternalIDs(ctx)
	if err != nil {
		t.Fatalf("ListExternalIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 external ids, got %d (%v)", len(ids), ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen["extA"] || !seen["extB"] {
		t.Fatalf("missing expected external IDs: %v", ids)
	}
}

func TestIndexInsertDuplicate(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	meta := app.Meta{Version: 1, NonceB64u: "dup"}
	if err := ix.Insert(ctx, store.NewRow{ID: "dup1", Meta: meta, Inline: []byte("a"), Size: 1, ManageHash: testManageHash, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := ix.Insert(ctx, store.NewRow{ID: "dup1", Meta: meta, Inline: []byte("b"), Size: 1, ManageHash: testManageHash, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err == nil {
		t.Fatalf("expected duplicate insert error")
	}
}

func TestIndexClaimMissing(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := ix.Claim(ctx, "nope", "hash", false, now, now.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIndexClaimBeginImmediateError(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := ix.Claim(ctx, "any", "hash", false, now, now.Add(time.Minute), nil); err == nil {
		t.Fatalf("expected error from closed DB")
	}
}

func TestIndexDeleteExpiredNone(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	recs, err := ix.DeleteExpired(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired empty: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 recs, got %d", len(recs))
	}
}

func TestIndexDeleteExpiredBeginTxError(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	db.Close()
	ctx := context.Background()
	if _, err := ix.DeleteExpired(ctx, time.Now()); err == nil {
		t.Fatalf("expected error on closed DB")
	}
}

func TestIndexListExternalIDsClosedDB(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	db.Close()
	ctx := context.Background()
	if _, err := ix.ListExternalIDs(ctx); err == nil {
		t.Fatalf("expected error querying closed DB")
	}
}

func TestIndexImmediateTx(t *testing.T) {
	sentinel := errors.New("boom")
	cases := []struct {
		name       string
		fnErr      error
		wantErr    error
		wantExists bool
	}{
		{name: "commit on nil", fnErr: nil, wantErr: nil, wantExists: false},
		{name: "commit and not found on dead row", fnErr: errDeadRow, wantErr: app.ErrNotFound, wantExists: false},
		{name: "rollback on not found", fnErr: app.ErrNotFound, wantErr: app.ErrNotFound, wantExists: true},
		{name: "rollback on other error", fnErr: sentinel, wantErr: sentinel, wantExists: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			ix, err := New(db)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx := context.Background()
			now := time.Now().UTC()
			insertInlineSecret(t, ctx, ix, "row", []byte("x"), now, now.Add(time.Hour))
			err = ix.immediateTx(ctx, func(conn *sql.Conn) error {
				if delErr := deleteSecret(ctx, conn, "row"); delErr != nil {
					t.Fatalf("delete: %v", delErr)
				}
				return tc.fnErr
			})
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got := rowExists(t, db, "row"); got != tc.wantExists {
				t.Fatalf("row exists = %v, want %v", got, tc.wantExists)
			}
		})
	}
}

func TestIndexImmediateTxRollsBackOnPanic(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	insertInlineSecret(t, ctx, ix, "row", []byte("x"), now, now.Add(time.Hour))
	func() {
		defer func() { _ = recover() }()
		_ = ix.immediateTx(ctx, func(conn *sql.Conn) error {
			_ = deleteSecret(ctx, conn, "row")
			panic("boom")
		})
	}()
	if !rowExists(t, db, "row") {
		t.Fatalf("expected row to survive rolled-back panic")
	}
	if _, err := ix.Claim(ctx, "row", "hash", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("expected lock released after panic, got %v", err)
	}
}

func TestAttachExternalReader(t *testing.T) {
	openErr := errors.New("open failed")
	cases := []struct {
		name       string
		external   bool
		opener     store.ExternalOpener
		wantErr    bool
		wantReader bool
	}{
		{name: "inline ignores opener", external: false, opener: nil},
		{name: "external without opener", external: true, opener: nil, wantErr: true},
		{name: "external open error", external: true, opener: func(string) (io.ReadCloser, error) { return nil, openErr }, wantErr: true},
		{name: "external opens", external: true, opener: func(string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
		}, wantReader: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := &store.IndexResult{External: tc.external}
			err := attachExternalReader(res, "id", tc.opener)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if (res.Reader != nil) != tc.wantReader {
				t.Fatalf("reader set = %v, want %v", res.Reader != nil, tc.wantReader)
			}
		})
	}
}

func TestHashMatches(t *testing.T) {
	cases := []struct {
		stored, presented string
		want              bool
	}{
		{"", "", false},
		{"", "abc", false},
		{"abc", "abd", false},
		{"abc", "abc", true},
	}
	for _, tc := range cases {
		if got := hashMatches(tc.stored, tc.presented); got != tc.want {
			t.Errorf("hashMatches(%q,%q) = %v, want %v", tc.stored, tc.presented, got, tc.want)
		}
	}
}
