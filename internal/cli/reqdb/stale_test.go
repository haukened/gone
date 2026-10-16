package reqdb

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ageLock creates a lock file last modified two hours ago and returns its
// path.
func ageLock(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, lockName)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestStaleLockOneWinner has two waiters race for a pre-aged lock many
// times and checks that they never hold the lock at the same time, which
// they would if one removed the other's fresh lock.
func TestStaleLockOneWinner(t *testing.T) {
	d, dir := openTest(t)
	for round := 0; round < 300; round++ {
		ageLock(t, dir)
		var holders, maxHolders atomic.Int32
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		start := make(chan struct{})
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				unlock, err := d.lock(context.Background())
				if err != nil {
					errs <- err
					return
				}
				n := holders.Add(1)
				for {
					m := maxHolders.Load()
					if n <= m || maxHolders.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(200 * time.Microsecond)
				holders.Add(-1)
				unlock()
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: %v", round, err)
		}
		if maxHolders.Load() != 1 {
			t.Fatalf("round %d: %d holders at once, want 1", round, maxHolders.Load())
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != FileName {
			t.Errorf("left behind %s", e.Name())
		}
	}
}

// TestBreakStaleSparesFreshLock checks that breakStale looks at the lock
// again under the break file: a lock that is fresh by then (another waiter
// broke the stale one and took its place) is left alone, and the break file
// is cleaned up.
func TestBreakStaleSparesFreshLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, lockName)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if breakStale(path) {
		t.Fatal("took a lock that is held and fresh")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fresh lock removed: %v", err)
	}
	if _, err := os.Stat(path + breakName); !os.IsNotExist(err) {
		t.Fatalf("break file left behind: %v", err)
	}
}

// TestBreakStaleTakesStaleLock checks the winner removes the stale lock,
// holds a new one, and leaves no break file.
func TestBreakStaleTakesStaleLock(t *testing.T) {
	dir := t.TempDir()
	path := ageLock(t, dir)
	if !breakStale(path) {
		t.Fatal("did not take the stale lock")
	}
	if fi, err := os.Stat(path); err != nil || time.Since(fi.ModTime()) > time.Minute {
		t.Fatalf("lock not renewed: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("left behind: %v", entries)
	}
}

// TestBreakStaleBreakFile checks that a break file held by a live waiter
// blocks other waiters, and that an old one from a dead waiter does not
// block the lock for good.
func TestBreakStaleBreakFile(t *testing.T) {
	dir := t.TempDir()
	path := ageLock(t, dir)
	brk := path + breakName
	if err := os.WriteFile(brk, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if breakStale(path) {
		t.Fatal("broke the lock while another waiter held the break file")
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(brk, old, old); err != nil {
		t.Fatal(err)
	}
	if breakStale(path) { // drops the abandoned break file
		t.Fatal("won while the abandoned break file was still there")
	}
	if !breakStale(path) {
		t.Fatal("abandoned break file blocks the lock for good")
	}
}

// TestBreakStaleErrors checks that an unusable directory means no lock and
// no panic.
func TestBreakStaleErrors(t *testing.T) {
	if breakStale(filepath.Join(t.TempDir(), "missing", lockName)) {
		t.Fatal("took a lock in a missing directory")
	}
}
