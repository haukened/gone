package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

// sqliteShortWriteWait shortens writeWait for the duration of a test.
func sqliteShortWriteWait(t *testing.T) {
	t.Helper()
	orig := writeWait
	writeWait = 20 * time.Millisecond
	t.Cleanup(func() { writeWait = orig })
}

// sqliteNewTestIndex opens an Index over a fresh test database.
func sqliteNewTestIndex(t *testing.T) *Index {
	t.Helper()
	db := sqliteOpenTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ix
}

func TestAcquireWriteWithoutGate(t *testing.T) {
	ix := &Index{}
	release, err := ix.acquireWrite(context.Background())
	if err != nil {
		t.Fatalf("acquireWrite: %v", err)
	}
	release()
}

func TestAcquireWriteServesInArrivalOrder(t *testing.T) {
	ix := &Index{writeGate: make(chan struct{}, 1)}
	ctx := context.Background()
	first, err := ix.acquireWrite(ctx)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	const waiters = 5
	order := make(chan int, waiters)
	for n := range waiters {
		go func() {
			release, err := ix.acquireWrite(ctx)
			if err != nil {
				t.Errorf("waiter %d: %v", n, err)
				return
			}
			order <- n
			release()
		}()
		// Give each waiter time to block on the gate before starting the next,
		// so arrival order is known.
		time.Sleep(5 * time.Millisecond)
	}
	first()
	for want := range waiters {
		if got := <-order; got != want {
			t.Fatalf("turn %d went to waiter %d", want, got)
		}
	}
}

func TestAcquireWriteTimesOutAsBusy(t *testing.T) {
	sqliteShortWriteWait(t)
	ix := &Index{writeGate: make(chan struct{}, 1)}
	ix.writeGate <- struct{}{}
	_, err := ix.acquireWrite(context.Background())
	if !errors.Is(err, app.ErrBusy) {
		t.Fatalf("err = %v, want app.ErrBusy", err)
	}
}

func TestAcquireWriteStopsWithContext(t *testing.T) {
	ix := &Index{writeGate: make(chan struct{}, 1)}
	ix.writeGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ix.acquireWrite(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestWritesReportBusyWhileGateHeld(t *testing.T) {
	sqliteShortWriteWait(t)
	ix := sqliteNewTestIndex(t)
	ctx := context.Background()
	now := time.Now()
	ix.writeGate <- struct{}{}
	defer func() { <-ix.writeGate }()

	writes := map[string]func() error{
		"insert": func() error {
			return ix.Insert(ctx, store.NewRow{ID: "busy", Inline: []byte("x"), Size: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
		},
		"claim": func() error {
			_, err := ix.Claim(ctx, "busy", "hash", false, now, now.Add(time.Minute), nil)
			return err
		},
		"delete expired": func() error {
			_, err := ix.DeleteExpired(ctx, now)
			return err
		},
	}
	for name, write := range writes {
		if err := write(); !errors.Is(err, app.ErrBusy) {
			t.Errorf("%s: err = %v, want app.ErrBusy", name, err)
		}
	}
}

// sqliteLockedIndex returns an Index whose database is write-locked by another
// connection pool, with a 1ms busy timeout so SQLite reports SQLITE_BUSY.
func sqliteLockedIndex(t *testing.T) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "locked.db")
	open := func() *sql.DB {
		db, err := sql.Open(DriverName, "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1)")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	ix, err := New(open())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	holder, err := open().Conn(context.Background())
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	if _, err = holder.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = holder.ExecContext(context.Background(), "ROLLBACK")
		_ = holder.Close()
	})
	return ix
}

func TestSQLiteBusyMapsToErrBusy(t *testing.T) {
	ix := sqliteLockedIndex(t)
	ctx := context.Background()
	now := time.Now()
	if err := ix.Insert(ctx, store.NewRow{ID: "locked", Inline: []byte("x"), Size: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); !errors.Is(err, app.ErrBusy) {
		t.Fatalf("insert: err = %v, want app.ErrBusy", err)
	}
	if _, err := ix.Ack(ctx, "locked", "hash"); !errors.Is(err, app.ErrBusy) {
		t.Fatalf("ack: err = %v, want app.ErrBusy", err)
	}
}

func TestBusyErrorPassesOtherErrorsThrough(t *testing.T) {
	if err := busyError(nil); err != nil {
		t.Fatalf("busyError(nil) = %v", err)
	}
	other := errors.New("boom")
	if err := busyError(other); !errors.Is(err, other) || errors.Is(err, app.ErrBusy) {
		t.Fatalf("busyError(other) = %v, want it unchanged and not busy", err)
	}
}
