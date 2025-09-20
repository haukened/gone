package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/haukened/gone/internal/app"
)

// openTestDB opens a transient SQLite database file in a temp dir with WAL enabled.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db?_busy_timeout=5000&cache=shared")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	return db
}

func TestIndexInsertAndConsumeInline(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "inline1"
	meta := app.Meta{Version: 1, NonceB64u: "nonceA"}
	inline := []byte("ciphertext-bytes")
	now := time.Now().UTC()
	expires := now.Add(5 * time.Minute)
	if err := ix.Insert(ctx, id, meta, inline, false, int64(len(inline)), now, expires); err != nil {
		t.Fatalf("Insert inline: %v", err)
	}
	// Consume
	gotMeta, gotInline, external, size, err := ix.Consume(ctx, id, now.Add(1*time.Second))
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if external {
		t.Fatalf("expected inline secret, got external=true")
	}
	if size != int64(len(inline)) {
		t.Fatalf("size mismatch")
	}
	if string(gotInline) != string(inline) {
		t.Fatalf("inline data mismatch")
	}
	if gotMeta.Version != meta.Version || gotMeta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch: %+v", gotMeta)
	}
	// Double consume should yield not found
	if _, _, _, _, err := ix.Consume(ctx, id, now.Add(2*time.Second)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second consume, got %v", err)
	}
}

func TestIndexInsertAndConsumeExternal(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext1"
	meta := app.Meta{Version: 2, NonceB64u: "nonceB"}
	now := time.Now().UTC()
	expires := now.Add(10 * time.Minute)
	if err := ix.Insert(ctx, id, meta, nil, true, 1234, now, expires); err != nil {
		t.Fatalf("Insert external: %v", err)
	}
	gotMeta, gotInline, external, size, err := ix.Consume(ctx, id, now.Add(1*time.Second))
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if !external {
		t.Fatalf("expected external=true")
	}
	if len(gotInline) != 0 {
		t.Fatalf("expected empty inline slice")
	}
	if size != 1234 {
		t.Fatalf("size mismatch")
	}
	if gotMeta.Version != meta.Version || gotMeta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
}

func TestIndexConsumeExpired(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "exp1"
	meta := app.Meta{Version: 1, NonceB64u: "nonceC"}
	now := time.Now().UTC()
	expires := now.Add(1 * time.Second)
	if err := ix.Insert(ctx, id, meta, []byte("x"), false, 1, now, expires); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	// Advance time beyond expiration
	if _, _, _, _, err := ix.Consume(ctx, id, now.Add(2*time.Second)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired secret, got %v", err)
	}
}

func TestIndexExpireBefore(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	// Insert 3 secrets: one expired external, one expired inline, one future
	if err := ix.Insert(ctx, "gone-ext", app.Meta{Version: 1, NonceB64u: "n1"}, nil, true, 50, now.Add(-10*time.Minute), now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("insert ext expired: %v", err)
	}
	if err := ix.Insert(ctx, "gone-inl", app.Meta{Version: 1, NonceB64u: "n2"}, []byte("abc"), false, 3, now.Add(-9*time.Minute), now.Add(-4*time.Minute)); err != nil {
		t.Fatalf("insert inl expired: %v", err)
	}
	if err := ix.Insert(ctx, "future", app.Meta{Version: 1, NonceB64u: "n3"}, []byte("f"), false, 1, now, now.Add(30*time.Minute)); err != nil {
		t.Fatalf("insert future: %v", err)
	}
	recs, err := ix.ExpireBefore(ctx, now)
	if err != nil {
		t.Fatalf("ExpireBefore: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 expired records, got %d (%+v)", len(recs), recs)
	}
	// Build map
	m := map[string]bool{}
	extMap := map[string]bool{}
	for _, r := range recs {
		m[r.ID] = true
		extMap[r.ID] = r.External
	}
	if !m["gone-ext"] || !m["gone-inl"] {
		t.Fatalf("missing expected IDs in recs: %+v", recs)
	}
	if !extMap["gone-ext"] {
		t.Fatalf("expected external flag for gone-ext")
	}
	if extMap["gone-inl"] {
		t.Fatalf("unexpected external flag for gone-inl")
	}
	// Ensure rows actually removed
	if _, _, _, _, err := ix.Consume(ctx, "gone-ext", now.Add(1*time.Second)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found for removed gone-ext")
	}
	if _, _, _, _, err := ix.Consume(ctx, "gone-inl", now.Add(1*time.Second)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found for removed gone-inl")
	}
	// Future one still there
	if _, _, _, _, err := ix.Consume(ctx, "future", now.Add(1*time.Second)); err != nil {
		t.Fatalf("future consume failed: %v", err)
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
	if err := ix.Insert(ctx, "inl", app.Meta{Version: 1, NonceB64u: "ni"}, []byte("d"), false, 1, now, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("insert inline: %v", err)
	}
	if err := ix.Insert(ctx, "extA", app.Meta{Version: 1, NonceB64u: "na"}, nil, true, 11, now, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("insert extA: %v", err)
	}
	if err := ix.Insert(ctx, "extB", app.Meta{Version: 1, NonceB64u: "nb"}, nil, true, 12, now, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("insert extB: %v", err)
	}
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
	ix, _ := New(db)
	ctx := context.Background()
	now := time.Now().UTC()
	meta := app.Meta{Version: 1, NonceB64u: "dup"}
	if err := ix.Insert(ctx, "dup1", meta, []byte("a"), false, 1, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := ix.Insert(ctx, "dup1", meta, []byte("b"), false, 1, now, now.Add(time.Minute)); err == nil {
		t.Fatalf("expected duplicate insert error")
	}
}

func TestIndexConsumeMissing(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, _, _, _, err := ix.Consume(ctx, "nope", now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIndexConsumeBeginTxError(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
	// Close DB to force BeginTx error
	db.Close()
	ctx := context.Background()
	if _, _, _, _, err := ix.Consume(ctx, "any", time.Now()); err == nil {
		t.Fatalf("expected error from BeginTx after close")
	}
}

func TestIndexExpireBeforeNone(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
	ctx := context.Background()
	now := time.Now().UTC()
	recs, err := ix.ExpireBefore(ctx, now)
	if err != nil {
		t.Fatalf("ExpireBefore empty: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 recs, got %d", len(recs))
	}
}

func TestIndexExpireBeforeBeginTxError(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
	db.Close()
	ctx := context.Background()
	if _, err := ix.ExpireBefore(ctx, time.Now()); err == nil {
		t.Fatalf("expected error on closed DB")
	}
}

func TestIndexListExternalIDsClosedDB(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
	db.Close()
	ctx := context.Background()
	if _, err := ix.ListExternalIDs(ctx); err == nil {
		t.Fatalf("expected error querying closed DB")
	}
}
