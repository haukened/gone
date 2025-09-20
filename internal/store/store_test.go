package store_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/filesystem"
	"github.com/haukened/gone/internal/store/sqlite"
)

// fixedClock implements app.Clock for deterministic tests.
type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

// openTestDB mirrors the sqlite test helper.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "store.db?_busy_timeout=5000&cache=shared")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	return db
}

// writeTempBlob writes a blob directly (helper for orphan tests).
func writeTempBlob(t *testing.T, dir, id string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, id+".blob")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}
}

func TestStoreSaveInlineAndConsume(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, _ := sqlite.New(db)
	blobDir := t.TempDir()
	bs, _ := filesystem.New(blobDir)
	st := store.New(ix, bs, clk, 64) // inlineMax large enough

	id := "inl1"
	meta := app.Meta{Version: 1, NonceB64u: "nonceA"}
	data := []byte("hello-inline")
	expires := now.Add(5 * time.Minute)
	if err := st.Save(ctx, id, meta, io.NopCloser(bytesReader(data)), int64(len(data)), expires); err != nil {
		t.Fatalf("Save inline: %v", err)
	}
	// Consume first time
	gotMeta, rc, size, err := st.Consume(ctx, id)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != string(data) {
		t.Fatalf("data mismatch got=%q", b)
	}
	if size != int64(len(data)) {
		t.Fatalf("size mismatch")
	}
	if gotMeta.Version != meta.Version || gotMeta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
	// Second consume should be not found
	if _, _, _, err = st.Consume(ctx, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound second consume, got %v", err)
	}
}

func TestStoreSaveExternalAndConsume(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, _ := sqlite.New(db)
	blobDir := t.TempDir()
	bs, _ := filesystem.New(blobDir)
	st := store.New(ix, bs, clk, 4) // inlineMax small to force external

	id := "ext1"
	meta := app.Meta{Version: 2, NonceB64u: "nonceB"}
	data := []byte("this-is-external-data")
	expires := now.Add(10 * time.Minute)
	if err := st.Save(ctx, id, meta, io.NopCloser(bytesReader(data)), int64(len(data)), expires); err != nil {
		t.Fatalf("Save external: %v", err)
	}
	// File should exist before consume
	if _, err := os.Stat(filepath.Join(blobDir, id+".blob")); err != nil {
		t.Fatalf("expected blob file: %v", err)
	}
	// Consume
	gotMeta, rc, size, err := st.Consume(ctx, id)
	if err != nil {
		t.Fatalf("Consume external: %v", err)
	}
	readData, _ := io.ReadAll(rc)
	if string(readData) != string(data) {
		t.Fatalf("payload mismatch")
	}
	if size != int64(len(data)) {
		t.Fatalf("size mismatch")
	}
	if gotMeta.Version != meta.Version || gotMeta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
	// Close triggers deletion
	if err := rc.Close(); err != nil {
		t.Fatalf("close(delete): %v", err)
	}
	// Blob should now be gone
	if _, err := os.Stat(filepath.Join(blobDir, id+".blob")); !os.IsNotExist(err) {
		t.Fatalf("expected blob removed, err=%v", err)
	}
	// Second consume -> not found
	if _, _, _, err := st.Consume(ctx, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound second consume, got %v", err)
	}
}

func TestStoreConsumeExpired(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, _ := sqlite.New(db)
	bs, _ := filesystem.New(t.TempDir())
	st := store.New(ix, bs, clk, 64)

	id := "exp1"
	meta := app.Meta{Version: 1, NonceB64u: "nC"}
	data := []byte("x")
	expires := now.Add(-1 * time.Minute) // already expired
	if err := st.Save(ctx, id, meta, io.NopCloser(bytesReader(data)), int64(len(data)), expires); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Consume should return ErrNotFound because store interprets expired rows.
	if _, _, _, err := st.Consume(ctx, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired consume, got %v", err)
	}
}

func TestStoreExpireBefore(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, _ := sqlite.New(db)
	blobDir := t.TempDir()
	bs, _ := filesystem.New(blobDir)
	st := store.New(ix, bs, clk, 4)

	// Insert: one expired external, one expired inline, one future
	if err := st.Save(ctx, "gone-ext", app.Meta{Version: 1, NonceB64u: "a"}, io.NopCloser(bytesReader([]byte("external-data"))), int64(len("external-data")), now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("save ext: %v", err)
	}
	if err := st.Save(ctx, "gone-inl", app.Meta{Version: 1, NonceB64u: "b"}, io.NopCloser(bytesReader([]byte("inl"))), 3, now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("save inl: %v", err)
	}
	if err := st.Save(ctx, "future", app.Meta{Version: 1, NonceB64u: "c"}, io.NopCloser(bytesReader([]byte("f"))), 1, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("save future: %v", err)
	}
	// Force external for first secret by size > inlineMax (already done) ensure blob exists
	if _, err := os.Stat(filepath.Join(blobDir, "gone-ext.blob")); err != nil {
		t.Fatalf("missing ext blob: %v", err)
	}
	count, err := st.ExpireBefore(ctx, now)
	if err != nil {
		t.Fatalf("ExpireBefore: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 expired removed, got %d", count)
	}
	// External blob should be deleted by cleanup
	if _, err := os.Stat(filepath.Join(blobDir, "gone-ext.blob")); !os.IsNotExist(err) {
		t.Fatalf("expected external blob removed by janitor, err=%v", err)
	}
	// Inline consume working for future
	if _, _, _, err := st.Consume(ctx, "future"); err != nil {
		t.Fatalf("future consume: %v", err)
	}
}

func TestStoreReconcileDeletesOrphan(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, _ := sqlite.New(db)
	blobDir := t.TempDir()
	bs, _ := filesystem.New(blobDir)
	st := store.New(ix, bs, clk, 4)

	// Write an orphan blob directly (no index row)
	writeTempBlob(t, blobDir, "orphan", []byte("zzz"))
	// Ensure List sees it after freshness window
	time.Sleep(1100 * time.Millisecond)
	if err := st.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Orphan should be gone
	if _, err := os.Stat(filepath.Join(blobDir, "orphan.blob")); !os.IsNotExist(err) {
		t.Fatalf("expected orphan removed, err=%v", err)
	}
}

// bytesReader helper (duplicated minimal impl to avoid test import cycles)
func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct{ b []byte }

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

// --- Construction / nil guard tests ---

// mockBlobStore minimal implementation for negative tests.
type mockBlobStore struct{}

func (m mockBlobStore) Write(_ string, _ io.Reader, _ int64) error { return nil }
func (m mockBlobStore) Consume(_ string) (io.ReadCloser, error) {
	return io.NopCloser(bytesReader([]byte("x"))), nil
}
func (m mockBlobStore) Delete(_ string) error   { return nil }
func (m mockBlobStore) List() ([]string, error) { return nil, nil }

// mockIndex minimal implementation for negative tests.
type mockIndex struct{}

func (m mockIndex) Insert(_ context.Context, _ string, _ app.Meta, _ []byte, _ bool, _ int64, _ time.Time, _ time.Time) error {
	return nil
}
func (m mockIndex) Consume(_ context.Context, _ string, _ time.Time) (*store.IndexResult, error) {
	return nil, app.ErrNotFound
}
func (m mockIndex) ExpireBefore(_ context.Context, _ time.Time) ([]store.ExpiredRecord, error) {
	return nil, nil
}
func (m mockIndex) ListExternalIDs(_ context.Context) ([]string, error) { return nil, nil }

// nil store pointer tests.
func TestStoreNilReceiverConsume(t *testing.T) {
	var s *store.Store
	if _, _, _, err := s.Consume(context.Background(), "any"); err == nil {
		t.Fatalf("expected error on nil store Consume")
	}
}

func TestStoreNilReceiverSave(t *testing.T) {
	var s *store.Store
	if err := s.Save(context.Background(), "id", app.Meta{}, bytesReader([]byte("a")), 1, time.Now()); err == nil {
		t.Fatalf("expected error on nil store Save")
	}
}

func TestStoreNilIndex(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	bs := mockBlobStore{}
	s := store.New(nil, bs, clk, 10)
	if _, _, _, err := s.Consume(context.Background(), "x"); err == nil {
		t.Fatalf("expected error with nil index")
	}
}

func TestStoreNilClock(t *testing.T) {
	ix := mockIndex{}
	bs := mockBlobStore{}
	// pass nil clock
	s := store.New(ix, bs, nil, 10)
	if err := s.Save(context.Background(), "x", app.Meta{}, bytesReader([]byte("a")), 1, time.Now()); err == nil {
		t.Fatalf("expected error with nil clock in Save")
	}
}

func TestStoreSaveNegativeSize(t *testing.T) {
	ix := mockIndex{}
	bs := mockBlobStore{}
	clk := fixedClock{now: time.Now()}
	s := store.New(ix, bs, clk, 10)
	if err := s.Save(context.Background(), "x", app.Meta{}, bytesReader([]byte("a")), -1, time.Now()); err == nil {
		t.Fatalf("expected error for negative size")
	}
}
