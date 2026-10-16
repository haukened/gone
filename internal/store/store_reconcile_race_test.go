package store_test

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

// storeHoldWriteLock takes SQLite's write lock on a separate connection so
// that index writes block in SQLite's busy handler, as they do while another
// writer holds the lock.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - f: store fixture whose database is locked.
//
// Returns: a function that releases the lock; calls after the first do nothing.
func storeHoldWriteLock(t *testing.T, f storeFixture) func() {
	t.Helper()
	conn, err := f.db.Conn(f.ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	if _, err = conn.ExecContext(f.ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("BEGIN IMMEDIATE: %v", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_, _ = conn.ExecContext(f.ctx, "ROLLBACK")
			_ = conn.Close()
		})
	}
}

// storeBackdateBlob sets a blob file's times a minute into the past, beyond
// the blob store's freshness window, as if it had been written long ago.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//   - id: secret id.
//
// Returns: none; failures abort the test.
func storeBackdateBlob(t *testing.T, dir, id string) {
	t.Helper()
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(filepath.Join(dir, id+".blob"), old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}

// storeAwaitBlob waits until the blob for id holds size bytes, so its
// payload has been written and no later write will touch its times.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//   - id: secret id.
//   - size: expected payload length.
//
// Returns: none; failures abort the test.
func storeAwaitBlob(t *testing.T, dir, id string, size int64) {
	t.Helper()
	start, p := time.Now(), filepath.Join(dir, id+".blob")
	for st, err := os.Stat(p); err != nil || st.Size() < size; st, err = os.Stat(p) {
		if time.Since(start) > 3*time.Second {
			t.Fatalf("blob never completed")
		}
		time.Sleep(time.Millisecond)
	}
}

// storeReconcileDuringWrite locks the index, starts write on data, runs
// Reconcile once the blob is complete (and backdated) but its row is not yet
// committed, and then releases the lock.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - f: store fixture.
//   - id: id of the payload being written.
//   - data: payload bytes passed to write as a reader.
//   - write: the Save or Fill under test.
//
// Returns: none; failures abort the test.
func storeReconcileDuringWrite(t *testing.T, f storeFixture, id string, data []byte, write func(r io.Reader) error) {
	t.Helper()
	release := storeHoldWriteLock(t, f)
	defer release()
	done := make(chan error, 1)
	go func() { done <- write(storeBytesReader(data)) }()
	storeAwaitBlob(t, f.blobDir, id, int64(len(data)))
	storeBackdateBlob(t, f.blobDir, id)
	if err := f.store.Reconcile(f.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("write: %v", err)
	}
	storeAssertBlobExists(t, f.blobDir, id)
}

func TestStoreReconcileKeepsBlobOfPendingSave(t *testing.T) {
	now := time.Now().UTC()
	f := storeNewFixture(t, now, 4)
	id := "88888888888888888888888888888888"
	data := []byte("external-payload")
	meta := app.Meta{Version: 1, NonceB64u: "n"}
	storeReconcileDuringWrite(t, f, id, data, func(r io.Reader) error {
		return f.store.Save(f.ctx, id, meta, storeTestManageHash, r, int64(len(data)), now.Add(time.Hour))
	})
	claimed, body := storeClaim(t, f, id, "claim-hash", false, now.Add(time.Minute))
	storeAssertClaim(t, claimed, body, data, meta, now.Add(time.Minute))
}

func TestStoreReconcileKeepsBlobOfPendingFill(t *testing.T) {
	now := time.Now().UTC()
	f := storeNewFixture(t, now, 4)
	reqCreate(t, f, now)
	reply := []byte("external-reply")
	storeReconcileDuringWrite(t, f, reqID, reply, func(r io.Reader) error {
		_, err := f.store.Fill(f.ctx, reqID, reqFillHash, app.Meta{Version: 3, NonceB64u: "n"}, r, int64(len(reply)))
		return err
	})
	if c, err := f.store.ClaimReply(f.ctx, reqID, reqManageHash, "claim", false, now.Add(time.Minute)); err != nil {
		t.Fatalf("ClaimReply: %v", err)
	} else {
		_ = c.Body.Close()
	}
}
