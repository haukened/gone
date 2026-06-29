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
	res, err := ix.Consume(ctx, id, now.Add(1*time.Second), nil)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if res.External {
		t.Fatalf("expected inline secret, got external=true")
	}
	if res.Size != int64(len(inline)) {
		t.Fatalf("size mismatch")
	}
	if string(res.Inline) != string(inline) {
		t.Fatalf("inline data mismatch")
	}
	if res.Meta.Version != meta.Version || res.Meta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch: %+v", res.Meta)
	}
	// Double consume should yield not found
	if _, err := ix.Consume(ctx, id, now.Add(2*time.Second), nil); !errors.Is(err, app.ErrNotFound) {
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
	res2, err := ix.Consume(ctx, id, now.Add(1*time.Second), func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if !res2.External {
		t.Fatalf("expected external=true")
	}
	if len(res2.Inline) != 0 {
		t.Fatalf("expected empty inline slice")
	}
	if res2.Reader == nil {
		t.Fatalf("expected external reader")
	}
	res2.Reader.Close()
	if res2.Size != 1234 {
		t.Fatalf("size mismatch")
	}
	if res2.Meta.Version != meta.Version || res2.Meta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
}

func TestIndexConsumeExternalOpenFailureLeavesRow(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext-open-failure"
	meta := app.Meta{Version: 2, NonceB64u: "nonceD"}
	now := time.Now().UTC()
	expires := now.Add(10 * time.Minute)
	if err := ix.Insert(ctx, id, meta, nil, true, 1234, now, expires); err != nil {
		t.Fatalf("Insert external: %v", err)
	}
	openErr := errors.New("open failed")
	if _, err := ix.Consume(ctx, id, now.Add(time.Second), func(string) (io.ReadCloser, error) {
		return nil, openErr
	}); !errors.Is(err, openErr) {
		t.Fatalf("expected open error, got %v", err)
	}
	res, err := ix.Consume(ctx, id, now.Add(2*time.Second), func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("expected retry consume to succeed, got %v", err)
	}
	if !res.External || res.Reader == nil {
		t.Fatalf("expected external result with reader")
	}
	res.Reader.Close()
}

func TestIndexConsumeExternalConcurrentOnlyOneSucceeds(t *testing.T) {
	db := openTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext-concurrent"
	now := time.Now().UTC()
	if err := ix.Insert(ctx, id, app.Meta{Version: 1, NonceB64u: "nonceE"}, nil, true, 4, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("Insert external: %v", err)
	}
	var opens int32
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := ix.Consume(ctx, id, now.Add(time.Second), func(string) (io.ReadCloser, error) {
				atomic.AddInt32(&opens, 1)
				time.Sleep(50 * time.Millisecond)
				return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
			})
			if err == nil && res != nil && res.Reader != nil {
				_ = res.Reader.Close()
			}
			errs <- err
		}()
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
			t.Fatalf("unexpected consume error: %v", err)
		}
	}
	if successes != 1 || notFound != 1 {
		t.Fatalf("expected one success and one not found, got success=%d not_found=%d", successes, notFound)
	}
	if got := atomic.LoadInt32(&opens); got != 1 {
		t.Fatalf("expected exactly one external open, got %d", got)
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
	// After expiry, index still returns the row (and deletes it) via DELETE RETURNING.
	res, err := ix.Consume(ctx, id, now.Add(2*time.Second), nil)
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected expired consume to return not found, got: %v", err)
	}
	if res != nil {
		t.Fatalf("expected no result for expired consume")
	}
	// Second consume is not found.
	if _, err := ix.Consume(ctx, id, now.Add(3*time.Second), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second consume, got %v", err)
	}
}

func TestIndexDeleteExpired(t *testing.T) {
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
	recs, err := ix.DeleteExpired(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
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
	if _, err := ix.Consume(ctx, "gone-ext", now.Add(1*time.Second), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found for removed gone-ext")
	}
	if _, err := ix.Consume(ctx, "gone-inl", now.Add(1*time.Second), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found for removed gone-inl")
	}
	// Future one still there
	if _, err := ix.Consume(ctx, "future", now.Add(1*time.Second), nil); err != nil {
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
	if _, err := ix.Consume(ctx, "nope", now, nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIndexConsumeBeginTxError(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
	// Close DB to force BeginTx error
	db.Close()
	ctx := context.Background()
	if _, err := ix.Consume(ctx, "any", time.Now(), nil); err == nil {
		t.Fatalf("expected error from BeginTx after close")
	}
}

func TestIndexDeleteExpiredNone(t *testing.T) {
	db := openTestDB(t)
	ix, _ := New(db)
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
	ix, _ := New(db)
	db.Close()
	ctx := context.Background()
	if _, err := ix.DeleteExpired(ctx, time.Now()); err == nil {
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
