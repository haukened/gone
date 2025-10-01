package janitor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// --- Fakes / Mocks ---

type fakeStore struct {
	mu          sync.Mutex
	expireCount int
	expireErr   error
	reconErr    error
	callsExpire int
	callsRecon  int
}

func (fs *fakeStore) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.callsExpire++
	if fs.expireErr != nil {
		return 0, fs.expireErr
	}
	return fs.expireCount, nil
}

func (fs *fakeStore) Reconcile(ctx context.Context) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.callsRecon++
	return fs.reconErr
}

func TestJanitorCycleSuccess(t *testing.T) {
	fs := &fakeStore{expireCount: 3}
	j := New(fs, nil, Config{Interval: time.Hour, Logger: slog.Default()})
	j.runCycle(context.Background())
	mv := j.MetricsSnapshot()
	if mv.Processed != 3 || mv.Deleted != 3 || mv.Cycles != 1 {
		t.Fatalf("unexpected metrics %+v", mv)
	}
	if fs.callsExpire != 1 || fs.callsRecon != 1 {
		t.Fatalf("expected one expire + one reconcile, got %d/%d", fs.callsExpire, fs.callsRecon)
	}
}

func TestJanitorCycleExpireError(t *testing.T) {
	fs := &fakeStore{expireErr: errors.New("boom")}
	j := New(fs, nil, Config{Interval: time.Hour, Logger: slog.Default()})
	j.runCycle(context.Background())
	mv := j.MetricsSnapshot()
	if mv.Processed != 0 || mv.Deleted != 0 || mv.Cycles != 1 {
		t.Fatalf("metrics after error %+v", mv)
	}
	if fs.callsRecon != 1 {
		t.Fatalf("expected reconcile even on expire error")
	}
}

func TestJanitorCycleReconcileError(t *testing.T) {
	fs := &fakeStore{expireCount: 2, reconErr: errors.New("r")}
	j := New(fs, nil, Config{Interval: time.Hour, Logger: slog.Default()})
	j.runCycle(context.Background())
	mv := j.MetricsSnapshot()
	if mv.Processed != 2 || mv.Deleted != 2 || mv.Cycles != 1 {
		t.Fatalf("metrics mismatch %+v", mv)
	}
	if fs.callsRecon != 1 {
		t.Fatalf("reconcile not called")
	}
}

func TestJanitorContextCancelEarly(t *testing.T) {
	fs := &fakeStore{expireCount: 5}
	j := New(fs, nil, Config{Interval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j.runCycle(ctx)
	mv := j.MetricsSnapshot()
	if mv.Processed != 5 {
		t.Fatalf("expected processed despite early cancel, got %d", mv.Processed)
	}
}

func TestStartStopLoop(t *testing.T) {
	fs := &fakeStore{expireCount: 1}
	j := New(fs, nil, Config{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	j.Start(ctx)
	time.Sleep(15 * time.Millisecond)
	j.Stop()
	cancel()
	mv := j.MetricsSnapshot()
	if mv.Cycles == 0 {
		t.Fatalf("expected at least one cycle")
	}
}

func TestNewDefaultsSimplified(t *testing.T) {
	fs := &fakeStore{}
	j := New(fs, nil, Config{})
	if j.cfg.Interval <= 0 || j.cfg.Logger == nil {
		t.Fatalf("defaults not applied %+v", j.cfg)
	}
}

func TestStartAlreadyStartedSimplified(t *testing.T) {
	fs := &fakeStore{}
	j := New(fs, nil, Config{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j.Start(ctx)
	tkr := j.ticker
	j.Start(ctx)
	if j.ticker != tkr {
		t.Fatalf("ticker replaced unexpectedly")
	}
	j.Stop()
}

// externalCollector captures emitted metrics for verification.
type externalCollector struct {
	mu       sync.Mutex
	counters map[string]int64
	observes map[string][]int64
}

func newExternalCollector() *externalCollector {
	return &externalCollector{counters: make(map[string]int64), observes: make(map[string][]int64)}
}

func (e *externalCollector) Inc(name string, delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.counters[name] += delta
}
func (e *externalCollector) Observe(name string, v int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.observes[name] = append(e.observes[name], v)
}

func TestJanitorExternalMetrics(t *testing.T) {
	fs := &fakeStore{expireCount: 4}
	ec := newExternalCollector()
	j := New(fs, ec, Config{Interval: time.Hour})
	j.runCycle(context.Background())
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.counters["secrets_expired_deleted_total"] != 4 {
		f := ec.counters["secrets_expired_deleted_total"]
		t.Fatalf("expected external counter 4 got %d", f)
	}
	obs := ec.observes["janitor_deleted_per_cycle"]
	if len(obs) != 1 || obs[0] != 4 {
		t.Fatalf("unexpected observations %+v", obs)
	}
}
