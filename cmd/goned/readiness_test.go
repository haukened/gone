package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a manually advanced time source.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}

// TestReadinessCache_SharesResultWithinTTL checks that calls inside the window
// run one probe and calls after it run another.
func TestReadinessCache_SharesResultWithinTTL(t *testing.T) {
	var calls atomic.Int32
	clk := &fakeClock{t: time.Unix(1000, 0)}
	c := &readinessCache{
		probe: func(context.Context) error { calls.Add(1); return nil },
		ttl:   2 * time.Second, now: clk.now,
	}
	for i := 0; i < 2; i++ {
		if err := c.check(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe ran %d times within the window, want 1", got)
	}
	clk.advance(3 * time.Second)
	if err := c.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("probe ran %d times after the window, want 2", got)
	}
}

// TestReadinessCache_CachesFailure checks that a failing result is reused too
// and that the probe context is detached from the caller's cancellation.
func TestReadinessCache_CachesFailure(t *testing.T) {
	var calls atomic.Int32
	boom := errors.New("down")
	clk := &fakeClock{t: time.Unix(1000, 0)}
	c := &readinessCache{
		probe: func(ctx context.Context) error {
			calls.Add(1)
			if ctx.Err() != nil {
				t.Error("probe context canceled by caller")
			}
			return boom
		},
		ttl: time.Second, now: clk.now,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 3; i++ {
		if err := c.check(ctx); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("probe ran %d times, want 1", calls.Load())
	}
}

// TestReadinessCache_ConcurrentSingleFlight checks that simultaneous callers
// share one probe run.
func TestReadinessCache_ConcurrentSingleFlight(t *testing.T) {
	var calls atomic.Int32
	c := &readinessCache{
		probe: func(context.Context) error { calls.Add(1); time.Sleep(20 * time.Millisecond); return nil },
		ttl:   time.Minute, now: time.Now,
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = c.check(context.Background()) }()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe ran %d times, want 1", got)
	}
}

// TestReadinessCheck_Cached checks the wired check reuses its result.
func TestReadinessCheck_Cached(t *testing.T) {
	db, _, blobDir := testStorage(t)
	check := readinessCheck(db, blobDir)
	if err := check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blobDir); err != nil {
		t.Fatal(err)
	}
	if err := check(context.Background()); err != nil {
		t.Fatalf("second call inside the window should reuse the result: %v", err)
	}
}

// TestCheckDirReadable covers empty, populated, missing, and non-directory paths.
func TestCheckDirReadable(t *testing.T) {
	dir := t.TempDir()
	if err := checkDirReadable(dir); err != nil {
		t.Fatalf("empty dir: %v", err)
	}
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkDirReadable(dir); err != nil {
		t.Fatalf("populated dir: %v", err)
	}
	if err := checkDirReadable(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing dir: want error")
	}
	if err := checkDirReadable(file); err == nil {
		t.Fatal("regular file: want error")
	}
}
